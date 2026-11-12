package e2e

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// stage is what every stage shares: the test, the fake and its workspace, the client's log, and the
// checks more than one stage makes. A stage embeds it, and its steps call the checks and return
// their own stage, so a scenario chains them with and().
type stage struct {
	// t is the test the scenario runs in.
	t *testing.T
	// assert checks an outcome and lets the scenario go on when it fails.
	assert *assert.Assertions
	// require checks what a step needs and ends the scenario when it fails.
	require *require.Assertions

	// fake is the Bitbucket the client calls.
	fake *fakebitbucket.Server
	// workspace is the workspace the scenario works in, once the stage seeded it.
	workspace models.Workspace
	// log holds the lines the logger wrote.
	log bytes.Buffer
}

// newStage returns a stage on a new, empty fake that t stops when the test ends.
func newStage(t *testing.T) stage {
	return stage{
		t:       t,
		assert:  assert.New(t),
		require: require.New(t),
		fake:    fakebitbucket.New(t),
	}
}

// logger returns a logger that writes every line to the stage's log, the debug ones included.
func (s *stage) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&s.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// checkALinePerAttempt checks that the log has one line per text in lines, in their order, each
// holding its text.
func (s *stage) checkALinePerAttempt(lines []string) {
	logged := strings.Split(strings.TrimSpace(s.log.String()), "\n")
	s.require.Len(logged, len(lines), "the log's lines")
	for i, line := range lines {
		s.assert.Contains(logged[i], line, "line %d of the log", i+1)
	}
}

// checkRequestCount checks that the fake got count requests for path.
func (s *stage) checkRequestCount(path string, count int) {
	s.assert.Len(requestsTo(s.fake, path), count, "the requests for %s", path)
}
