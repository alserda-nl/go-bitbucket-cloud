package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkAnswering returns a check that answers results in turn and counts its reads.
func checkAnswering(reads *int, results ...bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) {
		*reads++
		return results[min(*reads, len(results))-1], nil
	}
}

func TestVerify_ReadsUntilTheObjectMatches(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), checkAnswering(&reads, false, false, true), WithInterval(time.Millisecond))

	require.NoError(t, err)
	assert.Equal(t, 3, reads)
}

func TestVerify_ReadsOnceWhenTheFirstReadMatches(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), checkAnswering(&reads, true))

	require.NoError(t, err)
	assert.Equal(t, 1, reads)
}

func TestVerify_GivesUpAfterMaxAttempts(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), checkAnswering(&reads, false), WithInterval(0), WithMaxAttempts(4))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.EqualError(t, err, "bitbucket: the object did not read back as expected (reads: 4)")
	assert.Equal(t, 4, reads)
}

func TestVerify_ReadsAgainAfterWhatALaterReadCanChange(t *testing.T) {
	cases := []struct {
		name    string
		failure error
	}{
		{
			name:    "a 404",
			failure: &APIError{StatusCode: http.StatusNotFound},
		},
		{
			name:    "a 429",
			failure: &APIError{StatusCode: http.StatusTooManyRequests},
		},
		{
			name:    "a 503",
			failure: &APIError{StatusCode: http.StatusServiceUnavailable},
		},
		{
			name:    "a read that got no answer",
			failure: errors.New("connection refused"),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reads := 0

			err := Verify(t.Context(), func(context.Context) (bool, error) {
				reads++
				if reads < 3 {
					return false, testCase.failure
				}

				return true, nil
			}, WithInterval(0))

			require.NoError(t, err, "Verify should read again after %s", testCase.name)
			assert.Equal(t, 3, reads, "Verify should read again after %s", testCase.name)
		})
	}
}

func TestVerify_StopsAtARefusalNoLaterReadChanges(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), func(context.Context) (bool, error) {
		reads++
		return false, &APIError{StatusCode: http.StatusForbidden, Method: http.MethodGet, Path: "/2.0/a/path"}
	}, WithInterval(0))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.EqualError(t, err, "bitbucket: the object did not read back as expected (reads: 1): bitbucket: GET /2.0/a/path: 403 Forbidden")
	assert.Equal(t, 1, reads)
}

func TestVerify_NamesTheLastErrorWithoutWrappingIt(t *testing.T) {
	err := Verify(t.Context(), func(context.Context) (bool, error) {
		return false, &APIError{StatusCode: http.StatusNotFound, Method: http.MethodGet, Path: "/2.0/a/path"}
	}, WithInterval(0), WithMaxAttempts(2))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.NotErrorIs(t, err, ErrNotFound, "a 404 at the end must not look like a missing object")
	assert.EqualError(t, err, "bitbucket: the object did not read back as expected (reads: 2): bitbucket: GET /2.0/a/path: 404 Not Found")
}

func TestVerify_TreatsAnEndedContextAsReadsRunningOut(t *testing.T) {
	stopped := errors.New("the apply was stopped")
	cases := []struct {
		name  string
		check func(cancel context.CancelCauseFunc) func(context.Context) (bool, error)
	}{
		{
			name: "ended while waiting for the next read",
			check: func(cancel context.CancelCauseFunc) func(context.Context) (bool, error) {
				return func(context.Context) (bool, error) {
					time.AfterFunc(10*time.Millisecond, func() { cancel(stopped) })
					return false, nil
				}
			},
		},
		{
			name: "ended during a read, which then fails",
			check: func(cancel context.CancelCauseFunc) func(context.Context) (bool, error) {
				return func(ctx context.Context) (bool, error) {
					cancel(stopped)
					return false, ctx.Err()
				}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)

			err := Verify(ctx, testCase.check(cancel), WithInterval(time.Hour))

			assert.ErrorIs(t, err, ErrNotVerified, "Verify should end with ErrNotVerified when the context %s", testCase.name)
			assert.ErrorIs(t, err, stopped, "Verify should keep the cause the context %s with", testCase.name)
		})
	}
}

func TestVerify_ReadsOnceWithOneAttempt(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), checkAnswering(&reads, false), WithInterval(time.Hour), WithMaxAttempts(1))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.Equal(t, 1, reads, "Verify with one attempt should read once and not wait")
}

func TestVerify_WaitsTheIntervalBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	reads := 0

	err := Verify(ctx, func(context.Context) (bool, error) {
		reads++
		if reads == 1 {
			// The second read would come only after an hour: end the wait instead.
			time.AfterFunc(10*time.Millisecond, cancel)
		}

		return false, nil
	}, WithInterval(time.Hour))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.EqualError(t, err, "bitbucket: the object did not read back as expected (reads: 1): context canceled")
	assert.Equal(t, 1, reads)
}

func TestVerify_RefusesOptionsOutOfRange(t *testing.T) {
	cases := []struct {
		name    string
		options []VerifyOption
	}{
		{
			name:    "no reads",
			options: []VerifyOption{WithMaxAttempts(0)},
		},
		{
			name:    "a negative interval",
			options: []VerifyOption{WithInterval(-time.Second)},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reads := 0

			err := Verify(t.Context(), checkAnswering(&reads, true), testCase.options...)

			assert.ErrorIs(t, err, ErrInvalidOption, "Verify should refuse %s", testCase.name)
			assert.Zero(t, reads, "Verify should not read with %s", testCase.name)
		})
	}
}

func TestVerify_NamesTheMismatchTheCheckReported(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), func(context.Context) (bool, error) {
		reads++
		return false, fmt.Errorf("name is %q, want %q", "old", "new")
	}, WithInterval(0), WithMaxAttempts(3))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.EqualError(t, err, `bitbucket: the object did not read back as expected (reads: 3): name is "old", want "new"`)
	assert.Equal(t, 3, reads, "Verify should read again after a mismatch")
}
