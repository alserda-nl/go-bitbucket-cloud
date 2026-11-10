package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// midpoint pins the jitter to a factor of exactly 1.0 (0.8 + 0.4 * 0.5).
func midpoint() float64 {
	return 0.5
}

func newRetries(next http.RoundTripper, maxRetries int) *retryTransport {
	return &retryTransport{
		next:       next,
		maxRetries: maxRetries,
		firstWait:  time.Millisecond,
		randFloat:  midpoint,
	}
}

func TestRetryTransport_TriesAgainWhatTheAnswerAllows(t *testing.T) {
	cases := []struct {
		name             string
		method           string
		repeatable       bool
		statuses         []int
		expectedStatus   int
		expectedAttempts int
	}{
		{
			name:             "a GET after 500, 502, 503 and 504",
			method:           http.MethodGet,
			statuses:         []int{500, 502, 503, 504, 200},
			expectedStatus:   http.StatusOK,
			expectedAttempts: 5,
		},
		{
			name:             "a POST after a 429",
			method:           http.MethodPost,
			statuses:         []int{429, 201},
			expectedStatus:   http.StatusCreated,
			expectedAttempts: 2,
		},
		{
			name:             "a POST not after a 5xx, which can come after Bitbucket did the work",
			method:           http.MethodPost,
			statuses:         []int{500, 201},
			expectedStatus:   http.StatusInternalServerError,
			expectedAttempts: 1,
		},
		{
			name:             "a POST after a 5xx when it can be sent twice, as the token request",
			method:           http.MethodPost,
			repeatable:       true,
			statuses:         []int{503, 200},
			expectedStatus:   http.StatusOK,
			expectedAttempts: 2,
		},
		{
			name:             "a PUT after a 503",
			method:           http.MethodPut,
			statuses:         []int{503, 200},
			expectedStatus:   http.StatusOK,
			expectedAttempts: 2,
		},
		{
			name:             "nothing after a 404",
			method:           http.MethodGet,
			statuses:         []int{404, 200},
			expectedStatus:   http.StatusNotFound,
			expectedAttempts: 1,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			responses := make([]*http.Response, 0, len(testCase.statuses))
			for _, status := range testCase.statuses {
				responses = append(responses, stubResponse(status, nil, ""))
			}

			next := &recordingTransport{responses: responses}
			request, err := http.NewRequest(testCase.method, "https://api.bitbucket.org/2.0/thing", nil)
			require.NoError(t, err)

			retries := newRetries(next, 4)
			retries.repeatable = testCase.repeatable

			response, err := retries.RoundTrip(request)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedStatus, response.StatusCode, "RoundTrip(%s) answering %v returned the wrong answer", testCase.method, testCase.statuses)
			assert.Len(t, next.requests, testCase.expectedAttempts, "RoundTrip(%s) answering %v sent the wrong number of attempts", testCase.method, testCase.statuses)
		})
	}
}

func TestRetryTransport_StopsAfterMaxRetries(t *testing.T) {
	next := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusServiceUnavailable, nil, "")}}
	request, err := http.NewRequest(http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	response, err := newRetries(next, 2).RoundTrip(request)
	require.NoError(t, err)

	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	assert.Len(t, next.requests, 3)
}

func TestRetryTransport_NumbersTheAttemptsAndSendsTheBodyAgain(t *testing.T) {
	next := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusTooManyRequests, nil, ""),
		stubResponse(http.StatusOK, nil, ""),
	}}
	request, err := http.NewRequest(http.MethodPost, "https://api.bitbucket.org/2.0/thing", strings.NewReader(`{"a": 1}`))
	require.NoError(t, err)

	_, err = newRetries(next, 3).RoundTrip(request)
	require.NoError(t, err)

	assert.Equal(t, []string{`{"a": 1}`, `{"a": 1}`}, next.bodies)
	assert.Equal(t, 1, next.requests[0].Context().Value(attemptKey{}))
	assert.Equal(t, 2, next.requests[1].Context().Value(attemptKey{}))
}

func TestRetryTransport_ReturnsTheAnswerWhenTheBodyCannotBeSentAgain(t *testing.T) {
	cases := []struct {
		name    string
		getBody func() (io.ReadCloser, error)
	}{
		{
			name:    "no way to read it again",
			getBody: nil,
		},
		{
			name: "reading it again fails",
			getBody: func() (io.ReadCloser, error) {
				return nil, errors.New("gone")
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusTooManyRequests, nil, "")}}
			request, err := http.NewRequest(http.MethodPost, "https://api.bitbucket.org/2.0/thing", io.MultiReader(strings.NewReader("{}")))
			require.NoError(t, err)
			request.GetBody = testCase.getBody

			response, err := newRetries(next, 3).RoundTrip(request)
			require.NoError(t, err)

			assert.Equal(t, http.StatusTooManyRequests, response.StatusCode, "RoundTrip should return the 429 at once (%s)", testCase.name)
			assert.Len(t, next.requests, 1, "RoundTrip should send one attempt (%s)", testCase.name)
		})
	}
}

func TestRetryTransport_DoesNotTryAConnectionErrorAgain(t *testing.T) {
	attempts := 0
	next := roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("connection refused")
	})
	request, err := http.NewRequest(http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	_, err = newRetries(next, 3).RoundTrip(request)

	assert.EqualError(t, err, "connection refused")
	assert.Equal(t, 1, attempts)
}

func TestRetryTransport_StopsWaitingWhenTheContextEnds(t *testing.T) {
	next := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusServiceUnavailable, nil, "")}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	retries := newRetries(next, 3)
	retries.firstWait = time.Hour

	_, err = retries.RoundTrip(request)

	assert.ErrorIs(t, err, context.Canceled)
	assert.EqualError(t, err, "waiting 5s to send it again after a 503: context canceled")
}

func TestRetryTransport_WaitsWhatTheAnswerAsksFor(t *testing.T) {
	next := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusTooManyRequests, http.Header{"Retry-After": {"0"}}, ""),
		stubResponse(http.StatusOK, nil, ""),
	}}
	// The doubling wait would be 5 seconds: only the Retry-After's 0 fits the deadline.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	retries := newRetries(next, 3)
	retries.firstWait = time.Hour

	response, err := retries.RoundTrip(request)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Len(t, next.requests, 2)
}

func TestRetryTransport_ReturnsTheAnswerAtOnceAfterARetryAfterAboveAMinute(t *testing.T) {
	cases := []struct {
		name       string
		retryAfter string
	}{
		{
			name:       "61 seconds",
			retryAfter: "61",
		},
		{
			name:       "more seconds than a time.Duration holds",
			retryAfter: "9223372037",
		},
		{
			name:       "a date two minutes away",
			retryAfter: time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			next := &recordingTransport{responses: []*http.Response{
				stubResponse(http.StatusTooManyRequests, http.Header{"Retry-After": {testCase.retryAfter}}, ""),
				stubResponse(http.StatusOK, nil, ""),
			}}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
			require.NoError(t, err)

			response, err := newRetries(next, 3).RoundTrip(request)
			require.NoError(t, err)

			assert.Equal(t, http.StatusTooManyRequests, response.StatusCode, "RoundTrip should return the 429 at once after a Retry-After of %s", testCase.name)
			assert.Len(t, next.requests, 1, "RoundTrip should send one attempt after a Retry-After of %s", testCase.name)
		})
	}
}

func TestRetryTransport_WaitsARetryAfterOfAMinute(t *testing.T) {
	next := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusTooManyRequests, http.Header{"Retry-After": {"60"}}, ""),
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	_, err = newRetries(next, 3).RoundTrip(request)

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.EqualError(t, err, "waiting 1m0s to send it again after a 429: context deadline exceeded")
}

func TestRetryTransport_NamesALaterAttemptThatGotNoAnswer(t *testing.T) {
	attempts := 0
	client := newTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return stubResponse(http.StatusServiceUnavailable, nil, ""), nil
		}

		// A transport that is an http.Client of its own wraps its error in a *url.Error.
		return nil, &url.Error{
			Op:  "Get",
			URL: req.URL.String(),
			Err: io.EOF,
		}
	}))

	err := client.call(t.Context(), http.MethodGet, []string{"2.0", "thing"}, nil, nil, nil)

	assert.ErrorIs(t, err, io.EOF)
	assert.EqualError(t, err, "bitbucket: GET /2.0/thing: attempt 2, after a 503: EOF")
}

func TestRetryTransport_WaitFor(t *testing.T) {
	retries := newRetries(nil, 3)
	retries.firstWait = 200 * time.Millisecond

	cases := []struct {
		name       string
		attempt    int
		retryAfter string
		expected   time.Duration
	}{
		{
			name:     "the first wait",
			attempt:  1,
			expected: 200 * time.Millisecond,
		},
		{
			name:     "doubled per attempt",
			attempt:  3,
			expected: 800 * time.Millisecond,
		},
		{
			name:     "capped at five seconds",
			attempt:  10,
			expected: 5 * time.Second,
		},
		{
			name:     "capped when the doubling overflows",
			attempt:  70,
			expected: 5 * time.Second,
		},
		{
			name:       "Retry-After in seconds wins",
			attempt:    1,
			retryAfter: "7",
			expected:   7 * time.Second,
		},
		{
			name:       "a Retry-After date in the past is no wait",
			attempt:    1,
			retryAfter: "Wed, 21 Oct 2015 07:28:00 GMT",
			expected:   0,
		},
		{
			name:       "a Retry-After that is neither is ignored",
			attempt:    1,
			retryAfter: "soon",
			expected:   200 * time.Millisecond,
		},
		{
			name:       "a Retry-After too long for a time.Duration is its longest",
			attempt:    1,
			retryAfter: "9223372037",
			expected:   time.Duration(mostSeconds) * time.Second,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := stubResponse(http.StatusTooManyRequests, http.Header{"Retry-After": {testCase.retryAfter}}, "")

			actual := retries.waitFor(testCase.attempt, response)

			assert.Equal(t, testCase.expected, actual, "waitFor(%d, Retry-After %q)", testCase.attempt, testCase.retryAfter)
		})
	}
}

func TestRetryTransport_WaitsNothingWithAZeroFirstWait(t *testing.T) {
	retries := newRetries(nil, 3)
	retries.firstWait = 0
	response := stubResponse(http.StatusServiceUnavailable, nil, "")

	for attempt := 1; attempt <= 3; attempt++ {
		actual := retries.waitFor(attempt, response)

		assert.Zero(t, actual, "waitFor(%d) with a zero first wait", attempt)
	}
}

func TestRetryTransport_WaitsWithinTwentyPercent(t *testing.T) {
	retries := newRetries(nil, 3)
	retries.firstWait = time.Second
	response := stubResponse(http.StatusServiceUnavailable, nil, "")

	retries.randFloat = func() float64 { return 0 }
	shortest := retries.waitFor(1, response)
	retries.randFloat = func() float64 { return 0.999999 }
	longest := retries.waitFor(1, response)

	assert.Equal(t, 800*time.Millisecond, shortest)
	assert.InDelta(t, float64(1200*time.Millisecond), float64(longest), float64(time.Millisecond))
}

func TestRetryAfter_ReadsAFutureDate(t *testing.T) {
	date := time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)

	actual, ok := retryAfter(date)

	assert.True(t, ok)
	assert.InDelta(t, float64(time.Minute), float64(actual), float64(2*time.Second))
}

// roundTripFunc lets a function stand in for a transport.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// recordingTransport answers with its responses in turn, the last one again once they run out,
// and keeps every request and request body it got.
type recordingTransport struct {
	responses []*http.Response
	requests  []*http.Request
	bodies    []string
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

func TestRetryTransport_KeepsTheCauseTheContextEndedWith(t *testing.T) {
	next := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusServiceUnavailable, nil, "")}}
	ctx, cancel := context.WithCancelCause(t.Context())
	stopped := errors.New("the apply was stopped")
	cancel(stopped)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/thing", nil)
	require.NoError(t, err)

	retries := newRetries(next, 3)
	retries.firstWait = time.Hour

	_, err = retries.RoundTrip(request)

	assert.ErrorIs(t, err, stopped)
}

func TestRetryTransport_CapsALongFirstWait(t *testing.T) {
	retries := newRetries(nil, 3)
	retries.firstWait = 10 * time.Second

	actual := retries.waitFor(1, stubResponse(http.StatusServiceUnavailable, nil, ""))

	assert.Equal(t, 5*time.Second, actual, "waitFor(1) with a first wait of 10 s")
}
