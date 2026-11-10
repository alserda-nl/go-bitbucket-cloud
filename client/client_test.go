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

func TestNew_NamesARefusedAddressWithoutItsUserPasswordOrQuery(t *testing.T) {
	cases := []struct {
		name     string
		address  string
		secret   string
		expected string
	}{
		{
			name:     "a user and a password",
			address:  "https://user:s3cret@example.com",
			secret:   "s3cret",
			expected: `bitbucket: invalid client configuration: the endpoint "https://example.com" holds a user or password; give the credential to New instead`,
		},
		{
			name:     "a user without a password",
			address:  "https://someone@example.com/api",
			secret:   "someone",
			expected: `bitbucket: invalid client configuration: the endpoint "https://example.com/api" holds a user or password; give the credential to New instead`,
		},
		{
			name:     "a password with a space, which does not parse",
			address:  "https://user:pass word@example.com",
			secret:   "pass word",
			expected: "bitbucket: invalid client configuration: the endpoint is not an address: net/url: invalid userinfo",
		},
		{
			name:     "a query with a token",
			address:  "https://api.bitbucket.org?token=s3cret",
			secret:   "s3cret",
			expected: `bitbucket: invalid client configuration: the endpoint "https://api.bitbucket.org" is not an http or https address without a query`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := New(nil, WithEndpoint(testCase.address))

			require.ErrorIs(t, err, ErrInvalidConfig, "New should refuse an endpoint with %s", testCase.name)
			assert.NotContains(t, err.Error(), testCase.secret, "New's error should not show the endpoint's %s", testCase.name)
			assert.EqualError(t, err, testCase.expected, "New should name the endpoint with %s by its scheme, host and path", testCase.name)
		})
	}
}

func TestNew_RefusesANilCredentialPointer(t *testing.T) {
	cases := []struct {
		name       string
		credential Credential
		expected   string
	}{
		{
			name:       "a nil *APIToken",
			credential: (*APIToken)(nil),
			expected:   "bitbucket: invalid client configuration: the credential is a nil *client.APIToken",
		},
		{
			name:       "a nil *OAuthConsumer",
			credential: (*OAuthConsumer)(nil),
			expected:   "bitbucket: invalid client configuration: the credential is a nil *client.OAuthConsumer",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := New(testCase.credential)

			assert.ErrorIs(t, err, ErrInvalidConfig, "New should refuse %s", testCase.name)
			assert.EqualError(t, err, testCase.expected, "New should name %s", testCase.name)
		})
	}
}

func TestNew_RefusesARedirectThatWouldSendTheBodyElsewhere(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		location string
		expected string
	}{
		{
			name:     "a 307 to another host",
			status:   http.StatusTemporaryRedirect,
			location: "https://elsewhere.example.com/upload?signature=secret",
			expected: "bitbucket: PUT /2.0/thing: a 307 would send the request's body to https://elsewhere.example.com, off the client's endpoint",
		},
		{
			name:     "a 308 to the endpoint's host over http",
			status:   http.StatusPermanentRedirect,
			location: "http://api.bitbucket.org/2.0/thing",
			expected: "bitbucket: PUT /2.0/thing: a 308 would send the request's body to http://api.bitbucket.org, off the client's endpoint",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &recordingTransport{responses: []*http.Response{
				stubResponse(testCase.status, http.Header{"Location": {testCase.location}}, ""),
				stubResponse(http.StatusOK, nil, "{}"),
			}}
			client := newTestClient(t, transport)

			err := client.call(t.Context(), http.MethodPut, []string{"2.0", "thing"}, nil, map[string]string{"value": "secret"}, nil)

			assert.EqualError(t, err, testCase.expected, "call should refuse %s", testCase.name)
			assert.Len(t, transport.requests, 1, "call should not send the body on after %s", testCase.name)
		})
	}
}

func TestNew_FollowsARedirectThatKeepsTheBodyOnTheEndpoint(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusTemporaryRedirect, http.Header{"Location": {"https://api.bitbucket.org/2.0/moved"}}, ""),
		stubResponse(http.StatusOK, nil, "{}"),
	}}
	client := newTestClient(t, transport)

	err := client.call(t.Context(), http.MethodPut, []string{"2.0", "thing"}, nil, map[string]string{"value": "secret"}, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{`{"value":"secret"}`, `{"value":"secret"}`}, transport.bodies)
	assert.Equal(t, "/2.0/moved", transport.requests[1].URL.Path)
}

func TestNew_LeavesAnyOtherRedirectToTheCallersCheck(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusFound, http.Header{"Location": {"https://bbuseruploads.s3.amazonaws.com/log"}}, ""),
	}}
	checked := 0
	given := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			checked++
			return http.ErrUseLastResponse
		},
	}
	client, err := New(nil, WithHTTPClient(given))
	require.NoError(t, err)

	err = client.call(t.Context(), http.MethodGet, []string{"2.0", "thing"}, nil, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, checked, "the caller's CheckRedirect should decide on a redirect without a body")
	assert.Len(t, transport.requests, 1)
}

func TestNew_StopsAfterTenRedirectsWithoutACheckOfTheCallers(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusFound, http.Header{"Location": {"https://api.bitbucket.org/2.0/thing"}}, ""),
	}}
	client := newTestClient(t, transport)

	err := client.call(t.Context(), http.MethodGet, []string{"2.0", "thing"}, nil, nil, nil)

	assert.EqualError(t, err, "bitbucket: GET /2.0/thing: stopped after 10 redirects")
	assert.Len(t, transport.requests, 10)
}
