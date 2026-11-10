package client

import (
	"context"
	"net/http"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// ProjectsArea holds the calls of the Projects group of Atlassian's API reference.
type ProjectsArea struct {
	// client sends the calls.
	client *Client
}

// Get reads the project with projectKey in workspace: Get(ctx, "acme", "ABC"). The key is
// case-sensitive, and an API token needs the scope read:project:bitbucket. A project that is not
// there fails with an error matching ErrNotFound, and so does a private one read without a
// credential.
func (a *ProjectsArea) Get(ctx context.Context, workspace, projectKey string) (*models.Project, error) {
	return callFor[models.Project](ctx, a.client, http.MethodGet, apiPath("workspaces", workspace, "projects", projectKey), nil)
}
