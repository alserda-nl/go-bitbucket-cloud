package client

import (
	"log/slog"
	"net/http"
	"time"
)

// loggingTransport logs one line per attempt: the method, the path, the status, the attempt's
// number and how long it took until the answer's headers. Never the query, a header or a body: a
// pipeline cache's download address carries a signed token in its query, a variable's body
// carries its value, and a header the credential.
type loggingTransport struct {
	// next sends the request on.
	next http.RoundTripper
	// logger gets the lines.
	logger *slog.Logger
}

// RoundTrip sends the request and logs the attempt, at the level levelOf picks.
func (l *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	response, err := l.next.RoundTrip(req)

	attempt, _ := req.Context().Value(attemptKey{}).(int)
	attributes := []slog.Attr{
		slog.String("method", req.Method),
		slog.String("path", req.URL.EscapedPath()),
		slog.Int("attempt", attempt),
		slog.Duration("duration", time.Since(start)),
	}
	if err != nil {
		attributes = append(attributes, slog.String("error", withoutURL(err).Error()))
	} else {
		attributes = append(attributes, slog.Int("status", response.StatusCode))
	}

	l.logger.LogAttrs(req.Context(), levelOf(response, err), "bitbucket request", attributes...)

	return response, err
}

// levelOf returns Warn for an attempt that got no answer or a temporary status, a 429 or a 5xx, and
// Debug for any other.
func levelOf(response *http.Response, err error) slog.Level {
	if err != nil || temporary(response.StatusCode) {
		return slog.LevelWarn
	}

	return slog.LevelDebug
}
