package fakebitbucket_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
)

// TestAnswers sends a request to every API route the fake serves and, when FAKEBITBUCKET_ANSWERS
// names a file, writes the answers there as a list of operation, status and body. The maintainers
// compare that file with the answers Bitbucket gave, running this test by
// its name: its name, the variable and the three keys stay as they are. The token address is left
// out: no answer of Bitbucket's to compare it with is recorded.
func TestAnswers(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("ABC")))

	paths := []string{
		"/2.0/workspaces/acme",
		"/2.0/workspaces/%7B11111111-2222-4333-8444-555555555555%7D",
		"/2.0/workspaces/acme/projects",
		"/2.0/workspaces/acme/projects?pagelen=101",
		"/2.0/workspaces/acme/projects/ABC",
		"/2.0/workspaces/acme/projects/NOSUCHPROJECT",
	}
	answers := make([]map[string]any, 0, len(paths))
	answered := map[string]bool{}
	for _, path := range paths {
		answer := send(t, fake, http.MethodGet, path)
		requests := fake.Requests()
		route := requests[len(requests)-1].Route
		answered[route] = true

		// The body goes as it came, so an array or a string is compared too, not lost as null.
		body := json.RawMessage("null")
		if len(answer.raw) > 0 {
			require.True(t, json.Valid(answer.raw), "TestAnswers: %s answered a body that is not JSON", path)
			body = answer.raw
		}

		answers = append(answers, map[string]any{
			"operation": route,
			"status":    answer.status,
			"body":      body,
		})
	}

	for _, route := range fakebitbucket.RoutePatterns(fake) {
		if route != fmt.Sprintf("POST %s", fakebitbucket.TokenPath) {
			assert.True(t, answered[route], "TestAnswers sends no request to %s", route)
		}
	}

	if file := os.Getenv("FAKEBITBUCKET_ANSWERS"); file != "" {
		encoded, err := json.MarshalIndent(answers, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(file, encoded, 0o600)) //nolint:gosec // the file the caller named
	}
}
