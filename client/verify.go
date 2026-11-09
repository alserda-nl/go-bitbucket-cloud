package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultVerifyInterval and defaultVerifyMaxAttempts wait about a minute, in at most 20 requests
	// per object: a new project can answer 404 for 38 seconds and more after its create.
	defaultVerifyInterval    = 3 * time.Second
	defaultVerifyMaxAttempts = 20
)

// ErrNotVerified is Verify's error whenever it ends before the object read back as expected. Verify
// runs after a write that succeeded, so a caller reports this apart from a failed write.
var ErrNotVerified = errors.New("bitbucket: the object did not read back as expected")

// VerifyOption changes how Verify reads.
type VerifyOption func(*verifySettings)

type verifySettings struct {
	// interval is the time between two reads.
	interval time.Duration
	// maxAttempts is how many reads Verify makes at most.
	maxAttempts int
}

// WithInterval sets the time between two reads, 3 seconds by default.
func WithInterval(interval time.Duration) VerifyOption {
	return func(s *verifySettings) { s.interval = interval }
}

// WithMaxAttempts sets how many reads Verify makes at most, 20 by default.
func WithMaxAttempts(maxAttempts int) VerifyOption {
	return func(s *verifySettings) { s.maxAttempts = maxAttempts }
}

// Verify reads until check says the object is there as expected, at once and then every interval,
// at most maxAttempts reads, since Bitbucket can show an old state for a while after a write.
// check returns true for a match, or false, with an error naming what it found if it likes, to
// read again. A 4xx other than 404 and 429, a refused login or an option out of range ends the
// reads early. Without a match the error matches ErrNotVerified and names the last error unwrapped.
func Verify(ctx context.Context, check func(context.Context) (bool, error), opts ...VerifyOption) error {
	settings := verifySettings{
		interval:    defaultVerifyInterval,
		maxAttempts: defaultVerifyMaxAttempts,
	}
	for _, opt := range opts {
		opt(&settings)
	}

	if settings.interval < 0 || settings.maxAttempts < 1 {
		return fmt.Errorf("%w: verify needs an interval of 0 or more and at least 1 read, not %s and %d", ErrInvalidOption, settings.interval, settings.maxAttempts)
	}

	for attempt := 1; ; attempt++ {
		matched, err := check(ctx)
		if err == nil && matched {
			return nil
		}

		// A read the context ended is as good as the reads running out: the object was written.
		if ctx.Err() != nil {
			return fmt.Errorf("%w (reads: %d): %w", ErrNotVerified, attempt, context.Cause(ctx))
		}
		if attempt == settings.maxAttempts || (err != nil && !readAgain(err)) {
			return notVerified(attempt, err)
		}

		if err := sleep(ctx, settings.interval); err != nil {
			return fmt.Errorf("%w (reads: %d): %w", ErrNotVerified, attempt, err)
		}
	}
}

// readAgain reports whether a later read can answer otherwise than a read that failed with err: a
// 404, a 429, a 5xx and a read that got no answer can; a refused login and an option out of range
// cannot, nor can any other answer of 400 or above.
func readAgain(err error) bool {
	if errors.Is(err, ErrLogin) || errors.Is(err, ErrInvalidOption) {
		return false
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return true
	}

	return apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError
}

// notVerified is Verify's error when its reads end, naming the last read's error, if any, as text.
func notVerified(reads int, last error) error {
	if last == nil {
		return fmt.Errorf("%w (reads: %d)", ErrNotVerified, reads)
	}

	return fmt.Errorf("%w (reads: %d): %s", ErrNotVerified, reads, last.Error())
}
