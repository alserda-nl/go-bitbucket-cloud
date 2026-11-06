package client

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingTransport_LogsOneLinePerAttemptWithoutQueryHeadersOrBody(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	next := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusCreated, nil, "")}}
	logging := &loggingTransport{
		next:   next,
		logger: logger,
	}
	ctx := context.WithValue(t.Context(), attemptKey{}, 2)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.bitbucket.org/2.0/cache/%7Bu%7D?signature=secret", strings.NewReader(`{"value": "secret"}`))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer secret")

	_, err = logging.RoundTrip(request)
	require.NoError(t, err)

	line := output.String()
	assert.Equal(t, 1, strings.Count(line, "\n"))
	assert.Contains(t, line, "level=DEBUG msg=\"bitbucket request\" method=POST path=/2.0/cache/%7Bu%7D attempt=2 duration=")
	assert.Contains(t, line, "status=201")
	assert.NotContains(t, line, "secret")
	assert.NotContains(t, line, "Authorization")
}

func TestLoggingTransport_LogsAFailedAttempt(t *testing.T) {
	var output bytes.Buffer
	logging := &loggingTransport{
		next: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		}),
		logger: slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}
	request, err := http.NewRequest(http.MethodGet, "https://api.bitbucket.org/2.0/user", nil)
	require.NoError(t, err)

	_, err = logging.RoundTrip(request)

	assert.EqualError(t, err, "connection refused")
	assert.Contains(t, output.String(), `level=WARN`)
	assert.Contains(t, output.String(), `error="connection refused"`)
	assert.NotContains(t, output.String(), "status=")
}

func TestLevelOf_WarnsOfTheAttemptsTheRetriesAreFor(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		expected slog.Level
	}{
		{
			name:     "a 201",
			status:   http.StatusCreated,
			expected: slog.LevelDebug,
		},
		{
			name:     "a 404",
			status:   http.StatusNotFound,
			expected: slog.LevelDebug,
		},
		{
			name:     "a 429",
			status:   http.StatusTooManyRequests,
			expected: slog.LevelWarn,
		},
		{
			name:     "a 503",
			status:   http.StatusServiceUnavailable,
			expected: slog.LevelWarn,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := levelOf(stubResponse(testCase.status, nil, ""), nil)

			assert.Equal(t, testCase.expected, actual, "levelOf(%d)", testCase.status)
		})
	}
}
