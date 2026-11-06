package e2e

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

type loginStage struct {
	t       *testing.T
	assert  *assert.Assertions
	require *require.Assertions

	fake      *fakebitbucket.Server
	workspace models.Workspace
	apiClient *client.Client

	// readKey is the key of the project the WHEN step read.
	readKey     string
	project     *models.Project
	projects    []models.Project
	log         bytes.Buffer
	err         error
	failedReads atomic.Int32
}

func newLoginStage(t *testing.T) (*loginStage, *loginStage, *loginStage) {
	fake := fakebitbucket.New(t)
	stage := &loginStage{
		t:         t,
		assert:    assert.New(t),
		require:   require.New(t),
		fake:      fake,
		workspace: seedWorkspace(fake, "acme"),
	}

	return stage, stage, stage
}

func (s *loginStage) and() *loginStage { return s }

// --- GIVEN ---

func (s *loginStage) a_private_project(key string) *loginStage {
	seedProject(s.fake, s.workspace, key)
	return s
}

func (s *loginStage) a_public_project(key string) *loginStage {
	seedProject(s.fake, s.workspace, key, fakebitbucket.Project.WithPrivate(false))
	return s
}

func (s *loginStage) the_client_logs_in_with(credential client.Credential) *loginStage {
	s.apiClient = newClient(s.t, s.fake, credential)
	return s
}

func (s *loginStage) the_client_logs_in_with_a_log(credential client.Credential) *loginStage {
	logger := slog.New(slog.NewTextHandler(&s.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s.apiClient = newClient(s.t, s.fake, credential, client.WithLogger(logger))
	return s
}

func (s *loginStage) an_api_token_bitbucket_does_not_take() *loginStage {
	s.apiClient = newClient(s.t, s.fake, client.APIToken{
		Email: fakebitbucket.Email,
		Token: "revoked",
	})
	return s
}

func (s *loginStage) the_client_asks_for_its_token_at(path string) *loginStage {
	s.apiClient = newClient(s.t, s.fake, oauthConsumer(), client.WithOAuthTokenURL(fmt.Sprintf("%s%s", s.fake.URL, path)))
	return s
}

func (s *loginStage) bitbucket_answers_the_next_request_with(status int) *loginStage {
	s.fake.FailNext(status, 1)
	return s
}

func (s *loginStage) the_access_token_has_expired() *loginStage {
	s.fake.ExpireTokens()
	return s
}

// --- WHEN ---

func (s *loginStage) the_project_is_read(key string) *loginStage {
	s.readKey = key
	s.project, s.err = s.apiClient.Projects.Get(s.t.Context(), s.workspace.Slug, key)
	return s
}

func (s *loginStage) the_project_is_read_by_requests_at_once(key string, requests int) *loginStage {
	s.readKey = key
	var reads sync.WaitGroup
	for range requests {
		reads.Go(func() {
			if _, err := s.apiClient.Projects.Get(s.t.Context(), s.workspace.Slug, key); err != nil {
				s.failedReads.Add(1)
			}
		})
	}
	reads.Wait()
	return s
}

func (s *loginStage) the_projects_are_listed() *loginStage {
	s.projects, s.err = s.apiClient.Workspaces.ListProjects(s.t.Context(), s.workspace.Slug)
	return s
}

// --- THEN ---

func (s *loginStage) the_project_is_returned(key string) *loginStage {
	s.require.NoError(s.err)
	s.assert.Equal(key, s.project.Key)
	s.assert.Equal(s.workspace.Slug, s.project.Workspace.Slug)
	return s
}

func (s *loginStage) only_these_projects_are_listed(keys ...string) *loginStage {
	s.require.NoError(s.err)
	listed := make([]string, 0, len(s.projects))
	for _, project := range s.projects {
		listed = append(listed, project.Key)
	}
	s.assert.Equal(keys, listed)
	return s
}

func (s *loginStage) the_project_is_not_found() *loginStage {
	s.assert.ErrorIs(s.err, client.ErrNotFound)
	return s
}

func (s *loginStage) every_read_succeeded() *loginStage {
	s.assert.Zero(s.failedReads.Load())
	return s
}

func (s *loginStage) the_login_failed_without_reading_as_a_missing_project() *loginStage {
	s.assert.ErrorIs(s.err, client.ErrLogin)
	s.assert.NotErrorIs(s.err, client.ErrNotFound)
	return s
}

func (s *loginStage) the_credential_is_refused() *loginStage {
	var apiErr *client.APIError
	s.require.ErrorAs(s.err, &apiErr)
	s.assert.Equal(http.StatusUnauthorized, apiErr.StatusCode)
	return s
}

func (s *loginStage) every_api_request_carried(credential string) *loginStage {
	requests := requestsTo(s.fake, fmt.Sprintf("/2.0/workspaces/%s/projects/%s", s.workspace.Slug, s.readKey))
	s.require.NotEmpty(requests, "no request read the project %s", s.readKey)
	for _, request := range requests {
		s.assert.Equal(credential, request.Credential, "%s %s", request.Method, request.Path)
	}

	return s
}

func (s *loginStage) the_log_has_a_line_per_attempt(lines ...string) *loginStage {
	logged := strings.Split(strings.TrimSpace(s.log.String()), "\n")
	s.require.Len(logged, len(lines))
	for i, line := range lines {
		s.assert.Contains(logged[i], line)
	}

	return s
}

func (s *loginStage) the_token_address_was_asked(times int) *loginStage {
	s.assert.Len(requestsTo(s.fake, fakebitbucket.TokenPath), times)
	return s
}
