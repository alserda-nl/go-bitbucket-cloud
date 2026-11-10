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

func TestVerify_SendsEachReadOnce(t *testing.T) {
	// Verify's interval does the waiting, not the client's retries.
	transport := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusTooManyRequests, nil, "")}}
	client := newTestClient(t, transport)

	err := Verify(t.Context(), func(ctx context.Context) (bool, error) {
		_, err := client.Workspaces.Get(ctx, "acme")
		return err == nil, err
	}, WithInterval(0), WithMaxAttempts(3))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.Len(t, transport.requests, 3, "Verify's 3 reads of an endless 429 should send 3 requests")
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
			name:    "a 500",
			failure: &APIError{StatusCode: http.StatusInternalServerError},
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

func TestVerify_ReadsAgainAfterAMatchThatCameWithAnError(t *testing.T) {
	reads := 0

	err := Verify(t.Context(), func(context.Context) (bool, error) {
		reads++
		if reads == 1 {
			return true, errors.New("connection reset")
		}

		return true, nil
	}, WithInterval(0))

	require.NoError(t, err)
	assert.Equal(t, 2, reads, "Verify should read again after a match that came with an error")
}

func TestVerify_StopsAtARefusalNoLaterReadChanges(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		expected string
	}{
		{
			name:     "a 403",
			status:   http.StatusForbidden,
			expected: "bitbucket: the object did not read back as expected (reads: 1): bitbucket: GET /2.0/a/path: 403 Forbidden",
		},
		{
			name:     "a 501, which says the address is wrong",
			status:   http.StatusNotImplemented,
			expected: "bitbucket: the object did not read back as expected (reads: 1): bitbucket: GET /2.0/a/path: 501 Not Implemented",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reads := 0

			err := Verify(t.Context(), func(context.Context) (bool, error) {
				reads++
				return false, &APIError{
					StatusCode: testCase.status,
					Method:     http.MethodGet,
					Path:       "/2.0/a/path",
				}
			}, WithInterval(0))

			assert.ErrorIs(t, err, ErrNotVerified, "Verify should end without a match after %s", testCase.name)
			assert.EqualError(t, err, testCase.expected, "Verify should name %s as the last read's error", testCase.name)
			assert.Equal(t, 1, reads, "Verify should not read again after %s", testCase.name)
		})
	}
}

func TestVerify_NamesTheLastErrorWithoutWrappingIt(t *testing.T) {
	err := Verify(t.Context(), func(context.Context) (bool, error) {
		return false, &APIError{
			StatusCode: http.StatusNotFound,
			Method:     http.MethodGet,
			Path:       "/2.0/a/path",
		}
	}, WithInterval(0), WithMaxAttempts(2))

	assert.ErrorIs(t, err, ErrNotVerified)
	assert.NotErrorIs(t, err, ErrNotFound, "a 404 at the end must not look like a missing object")
	assert.EqualError(t, err, "bitbucket: the object did not read back as expected (reads: 2): bitbucket: GET /2.0/a/path: 404 Not Found")
}

func TestVerify_TreatsAnEndedContextAsReadsRunningOut(t *testing.T) {
	stopped := errors.New("the apply was stopped")
	cases := []struct {
		name    string
		check   func(cancel context.CancelCauseFunc) func(context.Context) (bool, error)
		options []VerifyOption
	}{
		{
			name: "ended while waiting for the next read",
			check: func(cancel context.CancelCauseFunc) func(context.Context) (bool, error) {
				return func(context.Context) (bool, error) {
					time.AfterFunc(10*time.Millisecond, func() { cancel(stopped) })
					return false, nil
				}
			},
			options: []VerifyOption{WithInterval(time.Hour)},
		},
		{
			// With one read, only the ended context can give the cause: the reads running out cannot.
			name: "ended during a read, which then fails",
			check: func(cancel context.CancelCauseFunc) func(context.Context) (bool, error) {
				return func(ctx context.Context) (bool, error) {
					cancel(stopped)
					return false, ctx.Err()
				}
			},
			options: []VerifyOption{WithInterval(time.Hour), WithMaxAttempts(1)},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)

			err := Verify(ctx, testCase.check(cancel), testCase.options...)

			assert.ErrorIs(t, err, ErrNotVerified, "Verify should end with ErrNotVerified when the context %s", testCase.name)
			assert.ErrorIs(t, err, stopped, "Verify should keep the cause the context %s with", testCase.name)
		})
	}
}

func TestVerify_NamesTheLastReadsErrorWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	err := Verify(ctx, func(context.Context) (bool, error) {
		// The second read would come only after an hour: end the wait instead.
		time.AfterFunc(10*time.Millisecond, cancel)
		return false, &APIError{
			StatusCode: http.StatusNotFound,
			Method:     http.MethodGet,
			Path:       "/2.0/a/path",
		}
	}, WithInterval(time.Hour))

	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrNotFound, "the last read's 404 must not look like a missing object")
	assert.EqualError(t, err, "bitbucket: the object did not read back as expected (reads: 1): context canceled; the last read: bitbucket: GET /2.0/a/path: 404 Not Found")
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

func TestVerify_StopsAtWhatTheClientRefusesToSend(t *testing.T) {
	cases := []struct {
		name string
		read func(ctx context.Context, client *Client) error
	}{
		{
			name: "a list option out of range",
			read: func(ctx context.Context, client *Client) error {
				_, err := client.Workspaces.ListProjects(ctx, "acme", WithPageSize(0))
				return err
			},
		},
		{
			name: "an empty project key",
			read: func(ctx context.Context, client *Client) error {
				_, err := client.Projects.Get(ctx, "acme", "")
				return err
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &recordingTransport{}
			client := newTestClient(t, transport)
			reads := 0

			err := Verify(t.Context(), func(ctx context.Context) (bool, error) {
				reads++
				return false, testCase.read(ctx, client)
			}, WithInterval(0))

			assert.ErrorIs(t, err, ErrNotVerified, "Verify should end without a match after %s", testCase.name)
			assert.Equal(t, 1, reads, "Verify should not read again after %s", testCase.name)
			assert.Empty(t, transport.requests, "Verify's read should send no request for %s", testCase.name)
		})
	}
}

func TestVerify_AFailedLoginEndsTheReadsOnlyWhenTheConsumerIsRefused(t *testing.T) {
	cases := []struct {
		name          string
		tokens        http.RoundTripper
		expectedReads int
	}{
		{
			name:          "a token address that refuses the consumer",
			tokens:        &recordingTransport{responses: []*http.Response{stubResponse(http.StatusUnauthorized, nil, "")}},
			expectedReads: 1,
		},
		{
			name:          "a token address that answers 503",
			tokens:        &recordingTransport{responses: []*http.Response{stubResponse(http.StatusServiceUnavailable, nil, "")}},
			expectedReads: 3,
		},
		{
			name: "a token address that does not answer",
			tokens: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}),
			expectedReads: 3,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newOAuthTestClient(t, testCase.tokens, &recordingTransport{responses: []*http.Response{stubResponse(http.StatusOK, nil, "{}")}})
			reads := 0

			err := Verify(t.Context(), func(ctx context.Context) (bool, error) {
				reads++
				_, err := client.Workspaces.Get(ctx, "acme")
				return err == nil, err
			}, WithInterval(0), WithMaxAttempts(3))

			assert.ErrorIs(t, err, ErrNotVerified, "Verify should end without a match after %s", testCase.name)
			assert.Equal(t, testCase.expectedReads, reads, "Verify read the wrong number of times after %s", testCase.name)
		})
	}
}
