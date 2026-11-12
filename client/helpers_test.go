package client

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tokenAddress is the token address the tests' OAuth consumers ask, New's default.
const tokenAddress = "https://bitbucket.org/site/oauth2/access_token"

// apiEndpoint is the endpoint the login tests' requests go to.
func apiEndpoint() *url.URL {
	endpoint, _ := url.Parse("https://api.bitbucket.org")

	return endpoint
}

// newTestClient makes a client whose requests go to transport, without retry waits.
func newTestClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()

	client, err := New(nil, WithHTTPClient(&http.Client{Transport: transport}), WithRetryWait(0))
	require.NoError(t, err)

	return client
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

// newRetries returns the retries around next, waiting a millisecond before the second attempt and
// without the jitter.
func newRetries(next http.RoundTripper, maxRetries int) *retryTransport {
	return &retryTransport{
		next:       next,
		maxRetries: maxRetries,
		firstWait:  time.Millisecond,
		randFloat:  midpoint,
	}
}

// midpoint pins the jitter to a factor of exactly 1.0 (0.8 + 0.4 * 0.5).
func midpoint() float64 {
	return 0.5
}

// roundTripFunc lets a function stand in for a transport.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// recordingTransport answers with its responses in turn, the last one again once they run out,
// and keeps every request and request body it got.
type recordingTransport struct {
	// responses are the answers it gives, in turn.
	responses []*http.Response
	// requests are the requests it got, in the order they came.
	requests []*http.Request
	// bodies are the bodies of the requests that had one.
	bodies []string
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		r.bodies = append(r.bodies, string(body))
	}
	r.requests = append(r.requests, req)

	response := r.responses[min(len(r.requests), len(r.responses))-1]
	response.Request = req

	return response, nil
}

// stubResponse is an answer with status, header, an empty one when it is nil, and body.
func stubResponse(status int, header http.Header, body string) *http.Response {
	if header == nil {
		header = http.Header{}
	}

	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// longResponse is a 200 whose body holds size zero bytes.
func longResponse(size int64) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(io.LimitReader(zeros{}, size)),
	}
}

// zeros is an endless answer body of zero bytes, made as they are read rather than held.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)

	return len(p), nil
}

// failingBody is an answer whose reading breaks off.
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

func (failingBody) Close() error {
	return nil
}
