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

// Get reads a project by its key, which is case-sensitive. Without a credential a private project
// answers 404.
func (a *ProjectsArea) Get(ctx context.Context, workspace, projectKey string) (*models.Project, error) {
	return callFor[models.Project](ctx, a.client, http.MethodGet, apiPath("workspaces", workspace, "projects", projectKey), nil)
}
