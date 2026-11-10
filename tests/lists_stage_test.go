package e2e

import (
	"fmt"
	"testing"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

type listStage struct {
	stage

	options []client.ListOption

	projects []models.Project
	err      error
}

func newListStage(t *testing.T) (*listStage, *listStage, *listStage) {
	s := &listStage{stage: newStage(t)}
	s.workspace = seedWorkspace(s.fake, "acme")

	return s, s, s
}

func (s *listStage) and() *listStage { return s }

func (s *listStage) n_projects_in_the_workspace(count int) *listStage {
	for range count {
		s.fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(s.workspace)))
	}

	return s
}

func (s *listStage) the_list_options_are(options ...client.ListOption) *listStage {
	s.options = options
	return s
}

func (s *listStage) the_projects_are_listed_in(workspace string) *listStage {
	s.projects, s.err = newClient(s.t, s.fake, apiToken()).Workspaces.ListProjects(s.t.Context(), workspace, s.options...)
	return s
}

func (s *listStage) n_projects_are_returned(count int) *listStage {
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
