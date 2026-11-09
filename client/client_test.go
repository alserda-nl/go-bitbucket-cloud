package client

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RefusesAnIncompleteConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		options []Option
	}{
		{
			name:    "an endpoint that is not an address",
			options: []Option{WithEndpoint("api.bitbucket.org")},
		},
		{
			name:    "an endpoint with a query",
			options: []Option{WithEndpoint("https://api.bitbucket.org?x=1")},
		},
		{
			name:    "an endpoint that does not parse",
			options: []Option{WithEndpoint("https://api.bitbucket.org/%zz")},
		},
		{
			name:    "a token address over another scheme",
			options: []Option{WithOAuthTokenURL("ftp://bitbucket.org/site/oauth2/access_token")},
		},
		{
			name:    "negative retries",
			options: []Option{WithMaxRetries(-1)},
		},
		{
			name:    "a negative retry wait",
			options: []Option{WithRetryWait(-time.Second)},
		},
		{
			name:    "an endpoint without a host",
			options: []Option{WithEndpoint("https://")},
		},
		{
			name:    "no HTTP client",
			options: []Option{WithHTTPClient(nil)},
		},
		{
			name:    "an endpoint over http on another machine",
			options: []Option{WithEndpoint("http://api.bitbucket.org")},
		},
		{
			name:    "a token address with a key and secret",
			options: []Option{WithOAuthTokenURL("https://key:secret@example.com/site/oauth2/access_token")},
		},
		{
			name:    "a token address over http on another machine",
			options: []Option{WithOAuthTokenURL("http://bitbucket.org/site/oauth2/access_token")},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := New(nil, testCase.options...)

			assert.ErrorIs(t, err, ErrInvalidConfig, "New should refuse %s", testCase.name)
		})
	}
}

func TestNew_TrimsTheEndpointsTrailingSlash(t *testing.T) {
	client, err := New(nil, WithEndpoint("https://bitbucket.example.com/api/"))
	require.NoError(t, err)

	actual, err := client.apiURL([]string{"2.0", "user"}, nil)
	require.NoError(t, err)

	assert.Equal(t, "https://bitbucket.example.com/api/2.0/user", actual.String())
}

func TestNew_WrapsTheGivenHTTPClientWithoutChangingIt(t *testing.T) {
	sent := 0
	given := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			sent++
			return stubResponse(http.StatusOK, nil, `{}`), nil
		}),
		Timeout: time.Minute,
	}

	client, err := New(APIToken{Email: "someone@example.com", Token: "token"}, WithHTTPClient(given))
	require.NoError(t, err)

	_, err = client.Workspaces.Get(t.Context(), "acme")
	require.NoError(t, err)

	assert.Equal(t, 1, sent)
	assert.IsType(t, roundTripFunc(nil), given.Transport, "New must not change the given client")
	assert.Equal(t, time.Minute, client.httpClient.Timeout)
}

func TestNew_KeepsADefaultTransportAProgramReplaced(t *testing.T) {
	// A test tool such as httpmock replaces http.DefaultTransport with a transport of its own.
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return stubResponse(http.StatusOK, nil, `{"slug": "acme"}`), nil
	})

	client, err := New(nil)
	require.NoError(t, err)

	workspace, err := client.Workspaces.Get(t.Context(), "acme")
	require.NoError(t, err)

	assert.Equal(t, "acme", workspace.Slug)
}

func TestNew_GivesEachAttemptSixtySecondsForTheHeaders(t *testing.T) {
	client, err := New(nil)
	require.NoError(t, err)

	retries, ok := client.httpClient.Transport.(*retryTransport)
	require.True(t, ok)
	base, ok := retries.next.(*http.Transport)
	require.True(t, ok)

	assert.Equal(t, 60*time.Second, base.ResponseHeaderTimeout)
}

func TestNew_SaysWhichPartOfACredentialIsEmpty(t *testing.T) {
	cases := []struct {
		name       string
		credential Credential
		expected   string
	}{
		{
			name:       "an API token without its email address",
			credential: APIToken{Token: "token"},
			expected:   "bitbucket: invalid client configuration: the API token's email address is empty",
		},
		{
			name:       "an API token without the token",
			credential: APIToken{Email: "someone@example.com"},
			expected:   "bitbucket: invalid client configuration: the API token itself is empty",
		},
		{
			name:       "an OAuth consumer without its key",
			credential: OAuthConsumer{Secret: "secret"},
			expected:   "bitbucket: invalid client configuration: the OAuth consumer's key is empty",
		},
		{
			name:       "an OAuth consumer without its secret",
			credential: OAuthConsumer{Key: "key"},
			expected:   "bitbucket: invalid client configuration: the OAuth consumer's secret is empty",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := New(testCase.credential)

			assert.ErrorIs(t, err, ErrInvalidConfig, "New should refuse %s", testCase.name)
			assert.EqualError(t, err, testCase.expected, "New should name the empty part of %s", testCase.name)
		})
	}
}

func TestNew_RefusesAnAddressWithAPasswordWithoutShowingIt(t *testing.T) {
	_, err := New(nil, WithEndpoint("https://user:secret@example.com"))

	assert.ErrorIs(t, err, ErrInvalidConfig)
	assert.NotContains(t, err.Error(), "secret", "the error should hide the address's password")
}
