package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

// longestWait caps the doubling wait between two attempts.
const longestWait = 5 * time.Second

// attemptKey carries an attempt's number in its request's context, for the log and for
// APIError.Attempts.
type attemptKey struct{}

// retryTransport tries a request again after a 429, 500, 502, 503 or 504, at most maxRetries
// times. It waits firstWait, doubling up to longestWait, plus or minus 20 percent; a Retry-After
// header, which Bitbucket does not document, wins when it comes. Connection errors are not tried again.
type retryTransport struct {
	// next sends each attempt.
	next http.RoundTripper
	// maxRetries is how often an answer that allows it is tried again.
	maxRetries int
	// firstWait is the wait before the second attempt, doubled for each one after it.
	firstWait time.Duration
	// repeatable says every request through it can be sent twice without harm, as the OAuth token
	// request can: a POST is then tried again after a 5xx too.
	repeatable bool
	// randFloat returns a number in [0, 1); tests pin it.
	randFloat func() float64
}

// RoundTrip sends the request, again while the answer allows it, and returns the last answer; a
// context that ends during a wait fails it.
func (r *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		response, err := r.next.RoundTrip(req.WithContext(context.WithValue(req.Context(), attemptKey{}, attempt)))
		if err != nil {
			return nil, err
		}
		if attempt > r.maxRetries || !r.retryable(req.Method, response.StatusCode) {
			return response, nil
		}

		// A body that cannot be sent again makes this answer the last.
		again, err := rewindRequest(req)
		if err != nil {
			return response, nil
		}

		wait := r.waitFor(attempt, response)
		drainAndClose(response.Body)
		if err := sleep(req.Context(), wait); err != nil {
			closeBody(again)
			return nil, fmt.Errorf("waiting to send it again after a %d: %w", response.StatusCode, err)
		}

		req = again
	}
}

// retryable reports whether an answer is worth another attempt. A POST is tried again after a 429
// only, unless the transport's requests are repeatable: a 5xx can come after Bitbucket did the
// work, and starting a pipeline twice runs a deploy twice.
func (r *retryTransport) retryable(method string, status int) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return method != http.MethodPost || r.repeatable
	default:
		return false
	}
}

// temporary reports whether a status blames Bitbucket or the moment rather than the request: a 429
// or a 5xx. retryable tries a narrower set again.
func temporary(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// waitFor returns the wait before the next attempt: Retry-After when the answer has a usable one,
// otherwise firstWait doubled per attempt, capped at longestWait, plus or minus 20 percent.
func (r *retryTransport) waitFor(attempt int, response *http.Response) time.Duration {
	if wait, ok := retryAfter(response.Header.Get("Retry-After")); ok {
		return wait
	}

	wait := min(r.firstWait, longestWait)
	for range attempt - 1 {
		wait = min(2*wait, longestWait)
	}

	return time.Duration(float64(wait) * (0.8 + 0.4*r.randFloat()))
}

// longestRetryAfter is the most seconds a time.Duration holds; a Retry-After above it is unusable.
const longestRetryAfter = math.MaxInt64 / int64(time.Second)

// retryAfter reads a Retry-After header in either of its forms: "120" is 2 minutes, and an HTTP
// date is the time until then, 0 once it has passed. It reports false for a header it cannot use.
func retryAfter(header string) (time.Duration, bool) {
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds >= 0 && seconds <= longestRetryAfter {
		return time.Duration(seconds) * time.Second, true
	}
	if date, err := http.ParseTime(header); err == nil {
		return max(time.Until(date), 0), true
	}

	return 0, false
}

// sleep waits for wait, or returns ctx's cause when ctx ends first.
func sleep(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

// rewindRequest returns req ready to send again: a copy with its body from the start, or req itself
// when it has no body. It fails when the body cannot be read again.
func rewindRequest(req *http.Request) (*http.Request, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return req, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("the request's body cannot be sent again")
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("rewinding the request's body: %w", err)
	}

	again := req.Clone(req.Context())
	again.Body = body

	return again, nil
}

// closeBody closes the body of a request that a RoundTrip does not pass on, as RoundTrip must.
func closeBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}

// drainLimit is as much of an answer as drainAndClose reads: an error page's rest, but not a long
// answer the next attempt would wait for.
const drainLimit = 4 << 10

// drainAndClose reads up to drainLimit of the rest of an answer and closes it, so the connection
// of a short answer serves the next attempt.
func drainAndClose(body io.ReadCloser) {
	_, _ = io.CopyN(io.Discard, body, drainLimit)
	_ = body.Close()
}
