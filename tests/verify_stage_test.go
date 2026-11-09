package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

type verifyStage struct {
	t       *testing.T
	assert  *assert.Assertions
	require *require.Assertions

	fake      *fakebitbucket.Server
	workspace models.Workspace
	options   []client.VerifyOption

	err error
}

func newVerifyStage(t *testing.T) (*verifyStage, *verifyStage, *verifyStage) {
	fake := fakebitbucket.New(t)
	stage := &verifyStage{
		t:         t,
		assert:    assert.New(t),
		require:   require.New(t),
		fake:      fake,
		workspace: seedWorkspace(fake, "acme"),
		options:   []client.VerifyOption{client.WithInterval(time.Millisecond)},
	}

	return stage, stage, stage
}

func (s *verifyStage) and() *verifyStage { return s }

func (s *verifyStage) a_new_project_that_answers_404_for_n_reads(key string, reads int) *verifyStage {
	s.fake.HideProject(seedProject(s.fake, s.workspace, key), reads)
	return s
}

func (s *verifyStage) verify_reads_at_most(maxAttempts int) *verifyStage {
	s.options = append(s.options, client.WithMaxAttempts(maxAttempts))
	return s
}

func (s *verifyStage) the_project_is_verified(key string) *verifyStage {
	apiClient := newClient(s.t, s.fake, apiToken())

	// A 404 reads again: Verify waits for a new project without the check saying so.
	s.err = client.Verify(s.t.Context(), func(ctx context.Context) (bool, error) {
		project, err := apiClient.Projects.Get(ctx, s.workspace.Slug, key)
		if err != nil {
			return false, err
		}

		return project.Key == key, nil
	}, s.options...)
	return s
}

func (s *verifyStage) the_project_read_back() *verifyStage {
	s.assert.NoError(s.err)
	return s
}

func (s *verifyStage) the_project_is_not_verified() *verifyStage {
	s.assert.ErrorIs(s.err, client.ErrNotVerified)
	return s
}

func (s *verifyStage) the_project_was_read_n_times(key string, count int) *verifyStage {
	s.assert.Len(requestsTo(s.fake, projectPath(s.workspace, key)), count)
	return s
}
