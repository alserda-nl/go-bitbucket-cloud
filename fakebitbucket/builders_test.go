package fakebitbucket_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

func TestNewWorkspace_StartsFromTheRecordedAnswer(t *testing.T) {
	workspace := fakebitbucket.NewWorkspace(fakebitbucket.Workspace.WithSlug("acme"))

	assert.Equal(t, "acme", workspace.Slug)
	assert.NotEqual(t, workspace.Slug, workspace.Name, "NewWorkspace should give a name of its own")
	assert.Equal(t, "workspace", workspace.Type)
	assert.Equal(t, models.ForkingModeInternalOnly, workspace.ForkingMode)
	assert.True(t, workspace.IsPrivate)
	assert.False(t, workspace.CreatedOn.IsZero())
	assert.Regexp(t, `^\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$`, workspace.UUID)
	assert.Equal(t, "https://api.bitbucket.org/2.0/workspaces/acme", workspace.Links.Self.Href)
	assert.Equal(t, "https://bitbucket.org/acme/", workspace.Links.HTML.Href)
}

func TestNewProject_StartsFromTheRecordedAnswer(t *testing.T) {
	workspace := fakebitbucket.NewWorkspace(fakebitbucket.Workspace.WithSlug("acme"))

	project := fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("ABC"))

	assert.Equal(t, "project", project.Type)
	assert.True(t, project.IsPrivate)
	assert.False(t, project.CreatedOn.IsZero())
	assert.Equal(t, workspace, project.Workspace)
	assert.Equal(t, "team", project.Owner.Type)
	assert.Equal(t, workspace.UUID, project.Owner.UUID)
	assert.Equal(t, "https://api.bitbucket.org/2.0/workspaces/acme/projects/ABC", project.Links.Self.Href)
	assert.Equal(t, `https://api.bitbucket.org/2.0/repositories/acme?q=project.key="ABC"`, project.Links.Repositories.Href)
}

func TestBuilders_GiveEveryCallItsOwnIDs(t *testing.T) {
	first := fakebitbucket.NewProject()
	second := fakebitbucket.NewProject()
	firstWorkspace := fakebitbucket.NewWorkspace()
	secondWorkspace := fakebitbucket.NewWorkspace()

	assert.NotEqual(t, first.UUID, second.UUID)
	assert.NotEqual(t, first.Key, second.Key)
	assert.NotEqual(t, first.Name, second.Name)
	assert.NotEqual(t, firstWorkspace.UUID, secondWorkspace.UUID)
	assert.NotEqual(t, firstWorkspace.Slug, secondWorkspace.Slug)
	assert.NotEqual(t, first.UUID, firstWorkspace.UUID)
}

func TestNewProject_OptionsCompose(t *testing.T) {
	project := fakebitbucket.NewProject(
		fakebitbucket.Project.WithKey("ABC"),
		fakebitbucket.Project.WithPrivate(false),
	)

	assert.Equal(t, "ABC", project.Key)
	assert.False(t, project.IsPrivate)
}
