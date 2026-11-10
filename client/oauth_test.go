package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tokenAddress = "https://bitbucket.org/site/oauth2/access_token"

// tokenServer stands in for the token address: it issues token-1, token-2 and so on.
type tokenServer struct {
	// expiresIn is how many seconds each token lasts.
	expiresIn int
	// grants counts the tokens issued.
	grants int
	// forms are the form bodies of the token requests it granted.
	forms []string
}

func (s *tokenServer) RoundTrip(req *http.Request) (*http.Response, error) {
	key, secret, _ := req.BasicAuth()
	if key != "key" || secret != "secret" {
		return stubResponse(http.StatusUnauthorized, http.Header{"Content-Type": {"application/json"}}, `{"error": "invalid_client", "error_description": "Invalid OAuth client credentials"}`), nil
	}

	form, _ := io.ReadAll(req.Body)
	s.forms = append(s.forms, string(form))
	s.grants++

	return stubResponse(http.StatusOK, nil, fmt.Sprintf(`{"access_token": "token-%d", "expires_in": %d, "token_type": "bearer"}`, s.grants, s.expiresIn)), nil
}

// newTestTokenSource returns the token source an OAuth consumer with the key "key" and the secret
// "secret" gets, asking tokenAddress for its tokens, with now as its clock.
func newTestTokenSource(transport http.RoundTripper, now func() time.Time) *tokenSource {
	tokens := newTokenSource(transport, tokenAddress, "key", "secret")
	tokens.now = now

	return tokens
}

func TestTokenSource_ARequestWaitingForTheTokenLeavesWhenItsContextEnds(t *testing.T) {
	fetching := make(chan struct{})
	release := make(chan struct{})
	tokens := newTestTokenSource(roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(fetching)
		<-release

		return stubResponse(http.StatusOK, nil, `{"access_token": "token", "expires_in": 3600}`), nil
	}), time.Now)

	var first sync.WaitGroup
	first.Go(func() {
		_, _ = tokens.current(t.Context())
	})
	<-fetching

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := tokens.current(ctx)
	close(release)
	first.Wait()

	assert.ErrorIs(t, err, context.Canceled)
	assert.EqualError(t, err, "waiting for another request's access token: context canceled")
}

func TestTokenSource_KeepsATokenUntilItExpires(t *testing.T) {
	server := &tokenServer{expiresIn: 3600}
	clock := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	tokens := newTestTokenSource(server, func() time.Time { return clock })

	first, err := tokens.current(t.Context())
	require.NoError(t, err)
	again, err := tokens.current(t.Context())
	require.NoError(t, err)

	clock = clock.Add(time.Hour)
	expired, err := tokens.current(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "token-1", first)
	assert.Equal(t, "token-1", again)
	assert.Equal(t, "token-2", expired)
	assert.Equal(t, []string{"grant_type=client_credentials", "grant_type=client_credentials"}, server.forms)
}

func TestTokenSource_RenewsOnlyATokenNobodyRenewedYet(t *testing.T) {
	server := &tokenServer{expiresIn: 3600}
	tokens := newTestTokenSource(server, time.Now)

	stale, err := tokens.current(t.Context())
	require.NoError(t, err)

	fresh, err := tokens.renew(t.Context(), stale)
	require.NoError(t, err)

	// A request that was refused together with the first renews the same stale token.
	renewedByAnother, err := tokens.renew(t.Context(), stale)
	require.NoError(t, err)

	assert.Equal(t, "token-2", fresh)
	assert.Equal(t, "token-2", renewedByAnother)
	assert.Equal(t, 2, server.grants)
}

func TestTokenSource_Fails(t *testing.T) {
	cases := []struct {
		name           string
		tokenURL       string
		transport      http.RoundTripper
		expected       string
		expectedStatus int
	}{
		{
			name:           "a token address that is not one",
			tokenURL:       "https://bitbucket.org/%zz",
			transport:      &tokenServer{},
			expected:       `logging in with the OAuth consumer failed: parse "https://bitbucket.org/%zz": invalid URL escape "%zz"`,
			expectedStatus: 0,
		},
		{
			name:     "a connection that fails",
			tokenURL: tokenAddress,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}),
			expected:       "logging in with the OAuth consumer failed: asking https://bitbucket.org/site/oauth2/access_token for an access token: connection refused",
			expectedStatus: 0,
		},
		{
			name:     "an answer that breaks off",
			tokenURL: tokenAddress,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       failingBody{},
				}, nil
			}),
			expected:       "logging in with the OAuth consumer failed: reading the access token from https://bitbucket.org/site/oauth2/access_token: connection reset by peer",
			expectedStatus: 0,
		},
		{
			name:     "a refused consumer",
			tokenURL: tokenAddress,
			transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				req.SetBasicAuth("key", "wrong")
				return (&tokenServer{}).RoundTrip(req)
			}),
			expected:       "logging in with the OAuth consumer failed: POST /site/oauth2/access_token: 401 Unauthorized: invalid_client: Invalid OAuth client credentials",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:     "an answer without a token",
			tokenURL: tokenAddress,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return stubResponse(http.StatusOK, nil, `{"expires_in": 3600}`), nil
			}),
			expected:       "logging in with the OAuth consumer failed: the token address answered without an access token",
			expectedStatus: http.StatusOK,
		},
		{
			name:     "an answer that is not JSON",
			tokenURL: tokenAddress,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return stubResponse(http.StatusOK, nil, "<html>"), nil
			}),
			expected:       "logging in with the OAuth consumer failed: decoding the token address's answer: invalid character '<' looking for beginning of value",
			expectedStatus: http.StatusOK,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			tokens := newTestTokenSource(testCase.transport, time.Now)
			tokens.tokenURL = testCase.tokenURL

			_, err := tokens.current(t.Context())

			assert.EqualError(t, err, testCase.expected, "current failed with the wrong error for %s", testCase.name)
			assert.ErrorIs(t, err, ErrLogin, "current should fail with ErrLogin for %s", testCase.name)

			var loginErr *loginError
			require.ErrorAs(t, err, &loginErr, "current should fail with a *loginError for %s", testCase.name)
			assert.Equal(t, testCase.expectedStatus, loginErr.status, "current kept the wrong status of the token address for %s", testCase.name)
		})
	}
}

func TestTokenSource_SaysHowOftenARefusedTokenRequestWasSent(t *testing.T) {
	retries := newRetries(&recordingTransport{responses: []*http.Response{stubResponse(http.StatusServiceUnavailable, nil, "")}}, 3)
	retries.repeatable = true
	tokens := newTestTokenSource(retries, time.Now)

	_, err := tokens.current(t.Context())

	assert.EqualError(t, err, "logging in with the OAuth consumer failed: POST /site/oauth2/access_token: 503 Service Unavailable (after 4 attempts)")
}

func TestBearerLogin_AFailedTokenRequestMatchesErrLoginThroughACall(t *testing.T) {
	// A transport that is an http.Client of its own wraps its error in a *url.Error.
	tokens := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, &url.Error{
			Op:  "Post",
			URL: req.URL.String(),
			Err: errors.New("connection refused"),
		}
	})
	client := newOAuthTestClient(t, tokens, &recordingTransport{responses: []*http.Response{stubResponse(http.StatusOK, nil, "{}")}})

	_, err := client.Workspaces.Get(t.Context(), "acme")

	assert.ErrorIs(t, err, ErrLogin)
	assert.EqualError(t, err, "bitbucket: GET /2.0/workspaces/acme: logging in with the OAuth consumer failed: asking https://bitbucket.org/site/oauth2/access_token for an access token: connection refused")
}

func TestTokenSource_ATokenAddressThatAnswers404IsNoObjectNotFound(t *testing.T) {
	// A wrong token address must not read as a missing object: a provider drops a resource from
	// state on a 404.
	tokens := newTestTokenSource(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return stubResponse(http.StatusNotFound, http.Header{"Content-Type": {"application/json"}}, `{"type": "error", "error": {"message": "Resource not found"}}`), nil
	}), time.Now)

	_, err := tokens.current(t.Context())

	assert.ErrorIs(t, err, ErrLogin)
	assert.NotErrorIs(t, err, ErrNotFound)
	assert.EqualError(t, err, "logging in with the OAuth consumer failed: POST /site/oauth2/access_token: 404 Not Found: Resource not found")
}

func TestBearerLogin_AsksForANewTokenOnceWhenBitbucketRefusesOne(t *testing.T) {
	server := &tokenServer{expiresIn: 3600}
	api := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusUnauthorized, nil, ""),
		stubResponse(http.StatusOK, nil, "{}"),
	}}
	login := newBearerLogin(api, server)
	request, err := http.NewRequest(http.MethodPut, "https://api.bitbucket.org/2.0/thing", strings.NewReader(`{"a": 1}`))
	require.NoError(t, err)

	response, err := login.RoundTrip(request)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "Bearer token-1", api.requests[0].Header.Get("Authorization"))
	assert.Equal(t, "Bearer token-2", api.requests[1].Header.Get("Authorization"))
	assert.Equal(t, []string{`{"a": 1}`, `{"a": 1}`}, api.bodies)
}

func TestBearerLogin_SendsNoTokenElsewhere(t *testing.T) {
	server := &tokenServer{expiresIn: 3600}
	api := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusOK, nil, "")}}
	login := newBearerLogin(api, server)
	request, err := http.NewRequest(http.MethodGet, "https://bbuseruploads.s3.amazonaws.com/log?Signature=x", nil)
	require.NoError(t, err)

	_, err = login.RoundTrip(request)
	require.NoError(t, err)

	assert.Empty(t, api.requests[0].Header.Get("Authorization"))
	assert.Zero(t, server.grants, "no token is asked for a request that gets none")
}

func TestTokenSource_FollowsNoRedirect(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusFound, http.Header{"Location": {"http://bitbucket.org/site/oauth2/access_token"}}, ""),
	}}
	tokens := newTestTokenSource(transport, time.Now)

	_, err := tokens.current(t.Context())

	assert.ErrorIs(t, err, ErrLogin)
	assert.EqualError(t, err, "logging in with the OAuth consumer failed: POST /site/oauth2/access_token: 302 Found")
	assert.Len(t, transport.requests, 1, "the consumer's key and secret go to the token address only")
}

func TestBearerLogin_ClosesTheBodyOfARequestItDoesNotSend(t *testing.T) {
	body := &closeCountingBody{Reader: strings.NewReader(`{"a": 1}`)}
	login := newBearerLogin(&recordingTransport{}, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	}))
	request, err := http.NewRequest(http.MethodPost, "https://api.bitbucket.org/2.0/thing", body)
	require.NoError(t, err)

	_, err = login.RoundTrip(request)

	require.Error(t, err)
	assert.Equal(t, 1, body.closed)
}

// closeCountingBody is a request body that counts how often it is closed.
type closeCountingBody struct {
	io.Reader
	closed int
}

func (b *closeCountingBody) Close() error {
	b.closed++

	return nil
}

func TestBearerLogin_GivesUp(t *testing.T) {
	cases := []struct {
		name           string
		tokens         http.RoundTripper
		body           io.Reader
		expectedStatus int
		expectedError  string
	}{
		{
			name: "when the first token cannot be had",
			tokens: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}),
			expectedError: "logging in with the OAuth consumer failed: asking https://bitbucket.org/site/oauth2/access_token for an access token: connection refused",
		},
		{
			name: "when the new token cannot be had",
			tokens: &recordingTransport{responses: []*http.Response{
				stubResponse(http.StatusOK, nil, `{"access_token": "token-1", "expires_in": 3600}`),
				stubResponse(http.StatusServiceUnavailable, nil, ""),
			}},
			expectedError: "logging in with the OAuth consumer failed: POST /site/oauth2/access_token: 503 Service Unavailable",
		},
		{
			name:           "with the 401 when the body cannot be sent again",
			tokens:         &tokenServer{expiresIn: 3600},
			body:           io.MultiReader(strings.NewReader("{}")),
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			api := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusUnauthorized, nil, "")}}
			login := newBearerLogin(api, testCase.tokens)
			request, err := http.NewRequest(http.MethodPost, "https://api.bitbucket.org/2.0/thing", testCase.body)
			require.NoError(t, err)

			response, err := login.RoundTrip(request)

			if testCase.expectedError != "" {
				assert.EqualError(t, err, testCase.expectedError, "RoundTrip should give up %s", testCase.name)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.expectedStatus, response.StatusCode, "RoundTrip should give up %s", testCase.name)
		})
	}
}

func newBearerLogin(api, tokens http.RoundTripper) http.RoundTripper {
	return OAuthConsumer{Key: "key", Secret: "secret"}.newLogin(loginSettings{
		next:           api,
		endpoint:       apiEndpoint(),
		tokenURL:       tokenAddress,
		tokenTransport: tokens,
	})
}

// newOAuthTestClient makes a client that logs in with an OAuth consumer, sending its token
// requests to tokens and every other request to api, without retry waits.
func newOAuthTestClient(t *testing.T, tokens, api http.RoundTripper) *Client {
	t.Helper()

	route := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == tokenAddress {
			return tokens.RoundTrip(req)
		}

		return api.RoundTrip(req)
	})

	client, err := New(OAuthConsumer{Key: "key", Secret: "secret"}, WithHTTPClient(&http.Client{Transport: route}), WithRetryWait(0))
	require.NoError(t, err)

	return client
}

// failingBody is an answer whose reading breaks off.
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

func (failingBody) Close() error {
	return nil
}

func TestBearerLogin_ReturnsAConnectionErrorAsItIs(t *testing.T) {
	server := &tokenServer{expiresIn: 3600}
	refused := errors.New("connection refused")
	login := newBearerLogin(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, refused
	}), server)
	request, err := http.NewRequest(http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	_, err = login.RoundTrip(request)

	assert.Equal(t, refused, err)
	assert.Equal(t, 1, server.grants)
}
