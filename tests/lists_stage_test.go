package e2e

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

type listStage struct {
	t       *testing.T
	assert  *assert.Assertions
	require *require.Assertions

	fake      *fakebitbucket.Server
	workspace models.Workspace
	options   []client.ListOption

	projects []models.Project
	err      error
}

func newListStage(t *testing.T) (*listStage, *listStage, *listStage) {
	fake := fakebitbucket.New(t)
	stage := &listStage{
		t:         t,
		assert:    assert.New(t),
		require:   require.New(t),
		fake:      fake,
		workspace: seedWorkspace(fake, "acme"),
	}

	return stage, stage, stage
}

func (s *listStage) and() *listStage { return s }

// --- GIVEN ---

func (s *listStage) projects_in_the_workspace(count int) *listStage {
	for range count {
		s.fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(s.workspace)))
	}

	return s
}

func (s *listStage) the_list_options_are(options ...client.ListOption) *listStage {
	s.options = options
	return s
}

// --- WHEN ---

func (s *listStage) the_projects_of_are_listed(workspace string) *listStage {
	s.projects, s.err = newClient(s.t, s.fake, apiToken()).Workspaces.ListProjects(s.t.Context(), workspace, s.options...)
	return s
}

// --- THEN ---

func (s *listStage) projects_are_returned(count int) *listStage {
	s.require.NoError(s.err)
	s.assert.Len(s.projects, count)
	return s
}

func (s *listStage) the_pages_asked_for_were(pagelens ...string) *listStage {
	requests := requestsTo(s.fake, fmt.Sprintf("/2.0/workspaces/%s/projects", s.workspace.Slug))
	asked := make([]string, 0, len(requests))
	for _, request := range requests {
		asked = append(asked, request.Query.Get("pagelen"))
	}
	s.assert.Equal(pagelens, asked)
	return s
}

func (s *listStage) the_option_is_refused_before_any_request() *listStage {
	s.assert.ErrorIs(s.err, client.ErrInvalidOption)
	s.assert.Empty(s.fake.Requests())
	return s
}

func (s *listStage) the_workspace_is_not_found() *listStage {
	s.assert.ErrorIs(s.err, client.ErrNotFound)
	return s
}
