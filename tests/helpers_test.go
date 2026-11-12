// Package e2e drives the client against the in-process fake over real HTTP, through the public
// API of client, models and fakebitbucket only. These tests carry no build tag and run in every
// make test.
//
// A scenario calls snake_case steps on its given, when and then stages, chained with and(), the
// first step on its stage's line and _n_ in a step that takes a count; verify_test.go is the shortest.
package e2e

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// apiToken is the API token the fake takes.
func apiToken() client.Credential {
	return client.APIToken{
		Email: fakebitbucket.Email,
		Token: fakebitbucket.APIToken,
	}
}

// oauthConsumer is the OAuth consumer the fake takes.
func oauthConsumer() client.Credential {
	return client.OAuthConsumer{
		Key:    fakebitbucket.OAuthKey,
		Secret: fakebitbucket.OAuthSecret,
	}
}

// noCredential is what a client that sends no credential is made with.
func noCredential() client.Credential {
	return nil
}

// newClient makes a client for the fake that logs in with credential and tries again without
// waiting.
func newClient(t *testing.T, fake *fakebitbucket.Server, credential client.Credential, opts ...client.Option) *client.Client {
	t.Helper()

	options := append([]client.Option{
		client.WithEndpoint(fake.URL),
		client.WithOAuthTokenURL(fmt.Sprintf("%s%s", fake.URL, fakebitbucket.TokenPath)),
		client.WithRetryWait(0),
	}, opts...)

	apiClient, err := client.New(credential, options...)
	require.NoError(t, err)

	return apiClient
}

// seedWorkspace stores a workspace with slug in the fake and returns it.
func seedWorkspace(fake *fakebitbucket.Server, slug string) models.Workspace {
	return fake.SeedWorkspace(fakebitbucket.NewWorkspace(fakebitbucket.Workspace.WithSlug(slug)))
}

// seedProject stores a project with key in workspace and returns it; opts change it further.
func seedProject(fake *fakebitbucket.Server, workspace models.Workspace, key string, opts ...fakebitbucket.ProjectOption) models.Project {
	return fake.SeedProject(fakebitbucket.NewProject(append([]fakebitbucket.ProjectOption{
		fakebitbucket.Project.WithWorkspace(workspace),
		fakebitbucket.Project.WithKey(key),
	}, opts...)...))
}

// requestsTo returns the requests the fake got for path.
func requestsTo(fake *fakebitbucket.Server, path string) []fakebitbucket.Request {
	return slices.DeleteFunc(fake.Requests(), func(request fakebitbucket.Request) bool {
		return request.Path != path
	})
}

// projectPath returns the API path of the project with key in workspace.
func projectPath(workspace models.Workspace, key string) string {
	return fmt.Sprintf("/2.0/workspaces/%s/projects/%s", workspace.Slug, key)
}
