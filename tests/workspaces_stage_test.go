package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

type workspaceStage struct {
	t       *testing.T
	assert  *assert.Assertions
	require *require.Assertions

	fake      *fakebitbucket.Server
	workspace models.Workspace
	login     client.Credential

	read *models.Workspace
	err  error
}

func newWorkspaceStage(t *testing.T) (*workspaceStage, *workspaceStage, *workspaceStage) {
	stage := &workspaceStage{
		t:       t,
		assert:  assert.New(t),
		require: require.New(t),
		fake:    fakebitbucket.New(t),
		login:   apiToken(),
	}

	return stage, stage, stage
}

func (s *workspaceStage) and() *workspaceStage { return s }

func (s *workspaceStage) a_private_workspace(slug string) *workspaceStage {
	s.workspace = seedWorkspace(s.fake, slug)
	return s
}

func (s *workspaceStage) the_client_logs_in_with(credential client.Credential) *workspaceStage {
	s.login = credential
	return s
}

func (s *workspaceStage) the_workspace_is_read(identifier string) *workspaceStage {
	s.read, s.err = newClient(s.t, s.fake, s.login).Workspaces.Get(s.t.Context(), identifier)
	return s
}

func (s *workspaceStage) the_workspace_is_read_by_its_uuid() *workspaceStage {
	return s.the_workspace_is_read(s.workspace.UUID)
}

func (s *workspaceStage) the_workspace_is_returned() *workspaceStage {
	s.require.NoError(s.err)
	s.assert.Equal(s.workspace.UUID, s.read.UUID)
	s.assert.Equal(s.workspace.Slug, s.read.Slug)
	s.assert.True(s.read.IsPrivate)
	return s
}

func (s *workspaceStage) the_workspace_is_not_found() *workspaceStage {
	s.assert.ErrorIs(s.err, client.ErrNotFound)
	return s
}
