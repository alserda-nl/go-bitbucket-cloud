package client

import (
	"context"
	"net/http"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// projectsLargestPage is the largest page the projects list takes: Bitbucket refuses 101 with 400
// "Invalid pagelen".
const projectsLargestPage = 100

// WorkspacesArea holds the calls of the Workspaces group of Atlassian's API reference.
type WorkspacesArea struct {
	// client sends the calls.
	client *Client
}

// Get reads a workspace by its slug in any case, Get(ctx, "acme"), or by its UUID in braces. A
// workspace that is not there fails with an error matching ErrNotFound. Bitbucket answers it
// without a credential too, for a private workspace as well.
func (a *WorkspacesArea) Get(ctx context.Context, workspace string) (*models.Workspace, error) {
	return callFor[models.Workspace](ctx, a.client, http.MethodGet, apiPath("workspaces", workspace), nil)
}

// ListProjects returns a workspace's projects, newest first as Bitbucket orders them: all of them,
// in pages of 100, unless a ListOption says otherwise. Without a credential it leaves out every
// private project. Atlassian's API reference lists this call in the Workspaces group only.
func (a *WorkspacesArea) ListProjects(ctx context.Context, workspace string, opts ...ListOption) ([]models.Project, error) {
	return list[models.Project](ctx, a.client, apiPath("workspaces", workspace, "projects"), projectsLargestPage, opts)
}
