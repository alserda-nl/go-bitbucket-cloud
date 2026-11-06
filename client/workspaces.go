package client

import (
	"context"
	"net/http"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// projectsLargestPage is the largest page the projects list takes: 101 answered 400 "Invalid
// pagelen" (2026-10-30).
const projectsLargestPage = 100

// WorkspacesArea holds the calls of the Workspaces group of Atlassian's API reference.
type WorkspacesArea struct {
	client *Client
}

// Get reads a workspace by its slug, in any case, or its UUID in braces. Bitbucket answers it
// without a credential too, for a private workspace as well (2026-10-30).
func (a *WorkspacesArea) Get(ctx context.Context, workspace string) (*models.Workspace, error) {
	return callFor[models.Workspace](ctx, a.client, http.MethodGet, apiPath("workspaces", workspace), nil)
}

// ListProjects lists a workspace's projects, newest created first, as Bitbucket gave them from
// 2026-10-12 to 2026-10-30; the client adds no sort. Atlassian's API reference lists this call in
// the Workspaces group only. Without a credential the list leaves out every private project
// (2026-10-30).
func (a *WorkspacesArea) ListProjects(ctx context.Context, workspace string, opts ...ListOption) ([]models.Project, error) {
	return list[models.Project](ctx, a.client, apiPath("workspaces", workspace, "projects"), projectsLargestPage, opts)
}
