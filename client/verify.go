package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultVerifyInterval and defaultVerifyMaxAttempts wait about a minute, longer than the 38
	// seconds a new project was seen answering 404, in at most 20 requests per object.
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

// Verify reads until check says the object is there as expected: the first read at once, then one
// every interval, at most maxAttempts reads. Bitbucket's answers cannot be trusted right after a
// write: a new project can answer 404 for 38 seconds and more.
//
// check returns true when the read matched, false to read again (a value not there yet), or an
// error. A check that finds a value other than the one written can return false with an error
// that names it ("name is \"old\", want \"new\""): Verify reads again, and names the last one. After a 404, which a new object answers for a while, a 429, a 5xx or a read that got no
// answer, Verify reads again; another answer of 400 or above, which no later read changes, ends the
// reads at once. When the reads end, the reads run out or ctx ends, Verify returns an error that
// matches ErrNotVerified. It names the last read's error but does not wrap it, so a 404 at the end
// is not taken for a missing object.
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
// 404, a 429 and a 5xx can, as can a read that got no answer.
func readAgain(err error) bool {
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
