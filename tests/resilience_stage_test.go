package e2e

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// resilienceStage sets the client's retries and logging in GIVEN steps and builds the client in the
// WHEN step, so each scenario drives its own configuration against a fake primed with FailNext.
type resilienceStage struct {
	t       *testing.T
	assert  *assert.Assertions
	require *require.Assertions

	fake      *fakebitbucket.Server
	workspace models.Workspace
	options   []client.Option
	log       bytes.Buffer

	// readKey is the key of the project the WHEN step read.
	readKey string
	project *models.Project
	err     error
}

func newResilienceStage(t *testing.T) (*resilienceStage, *resilienceStage, *resilienceStage) {
	fake := fakebitbucket.New(t)
	stage := &resilienceStage{
		t:         t,
		assert:    assert.New(t),
		require:   require.New(t),
		fake:      fake,
		workspace: seedWorkspace(fake, "acme"),
	}

	return stage, stage, stage
}

func (s *resilienceStage) and() *resilienceStage { return s }

func (s *resilienceStage) a_project(key string) *resilienceStage {
	seedProject(s.fake, s.workspace, key)
	return s
}

func (s *resilienceStage) the_client_tries_again_at_most(maxRetries int) *resilienceStage {
	s.options = append(s.options, client.WithMaxRetries(maxRetries))
	return s
}

func (s *resilienceStage) bitbucket_answers_the_next_n_requests_with(times, status int) *resilienceStage {
	s.fake.FailNext(status, times)
	return s
}

func (s *resilienceStage) the_project_is_read(key string) *resilienceStage {
	logger := slog.New(slog.NewTextHandler(&s.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	apiClient := newClient(s.t, s.fake, apiToken(), append(s.options, client.WithLogger(logger))...)

	s.readKey = key
	s.project, s.err = apiClient.Projects.Get(s.t.Context(), s.workspace.Slug, key)
	return s
}

func (s *resilienceStage) the_project_is_returned(key string) *resilienceStage {
	s.require.NoError(s.err)
	s.assert.Equal(key, s.project.Key)
	return s
}

func (s *resilienceStage) bitbucket_answer_is_returned(status int) *resilienceStage {
	var apiErr *client.APIError
	s.require.ErrorAs(s.err, &apiErr)
	s.assert.Equal(status, apiErr.StatusCode)
	return s
}

func (s *resilienceStage) the_project_was_requested_n_times(count int) *resilienceStage {
	s.assert.Len(requestsTo(s.fake, fmt.Sprintf("/2.0/workspaces/%s/projects/%s", s.workspace.Slug, s.readKey)), count)
	return s
}

func (s *resilienceStage) the_log_has_a_line_per_attempt(lines ...string) *resilienceStage {
	logged := strings.Split(strings.TrimSpace(s.log.String()), "\n")
	s.require.Len(logged, len(lines))
	for i, line := range lines {
		s.assert.Contains(logged[i], line)
	}

	return s
}
