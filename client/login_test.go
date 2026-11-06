package client

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicLogin_SendsTheTokenOnlyToTheEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		address  string
		ctx      context.Context
		expected bool
	}{
		{
			name:     "a call to the endpoint",
			address:  "https://api.bitbucket.org/2.0/user",
			ctx:      context.Background(),
			expected: true,
		},
		{
			name:     "the endpoint in other letters",
			address:  "https://API.Bitbucket.org/2.0/user",
			ctx:      context.Background(),
			expected: true,
		},
		{
			name:     "the endpoint's host over http",
			address:  "http://api.bitbucket.org/2.0/user",
			ctx:      context.Background(),
			expected: false,
		},
		{
			name:     "a redirect to a signed Amazon S3 address",
			address:  "https://bbuseruploads.s3.amazonaws.com/log?Signature=x",
			ctx:      context.Background(),
			expected: false,
		},
		{
			name:     "a call Bitbucket refuses with every API token",
			address:  "https://api.bitbucket.org/2.0/hook_events/workspace",
			ctx:      withoutCredential(context.Background()),
			expected: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusOK, nil, "")}}
			login := APIToken{Email: "someone@example.com", Token: "token"}.newLogin(loginSettings{
				next:     transport,
				endpoint: apiEndpoint(),
			})
			request, err := http.NewRequestWithContext(testCase.ctx, http.MethodGet, testCase.address, nil)
			require.NoError(t, err)

			_, err = login.RoundTrip(request)
			require.NoError(t, err)

			email, token, actual := transport.requests[0].BasicAuth()
			assert.Equal(t, testCase.expected, actual, "RoundTrip(%s) decided wrongly whether to send the token", testCase.address)
			if actual {
				assert.Equal(t, "someone@example.com", email)
				assert.Equal(t, "token", token)
			}

			assert.Empty(t, request.Header.Get("Authorization"), "RoundTrip(%s) changed the caller's request", testCase.address)
		})
	}
}

// apiEndpoint is the endpoint the login tests' requests go to.
func apiEndpoint() *url.URL {
	endpoint, _ := url.Parse("https://api.bitbucket.org")

	return endpoint
}

func TestCredentials_HideTheirSecretWhenPrintedOrLogged(t *testing.T) {
	cases := []struct {
		name       string
		credential Credential
	}{
		{
			name: "an API token",
			credential: APIToken{
				Email: "someone@example.com",
				Token: "s3cret",
			},
		},
		{
			name: "an OAuth consumer",
			credential: OAuthConsumer{
				Key:    "key",
				Secret: "s3cret",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var logged bytes.Buffer
			slog.New(slog.NewJSONHandler(&logged, nil)).Info("logging in", "credential", testCase.credential)

			printed := fmt.Sprintf("%v %+v %#v %s", testCase.credential, testCase.credential, testCase.credential, testCase.credential)

			assert.NotContains(t, printed, "s3cret", "printing %s should hide its secret", testCase.name)
			assert.Contains(t, printed, hidden, "printing %s should say the secret is hidden", testCase.name)
			assert.NotContains(t, logged.String(), "s3cret", "logging %s should hide its secret", testCase.name)
		})
	}
}
