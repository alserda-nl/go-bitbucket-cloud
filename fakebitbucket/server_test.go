package fakebitbucket_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// answer is a fake's answer: its status and decoded body.
type answer struct {
	status int
	// body is the answer decoded when it is a JSON object; raw is the answer as it came.
	body map[string]any
	raw  []byte
}

// send sends a request to the fake as the API token's holder, or with the given Authorization.
func send(t *testing.T, fake *fakebitbucket.Server, method, path string, authorization ...string) answer {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, fmt.Sprintf("%s%s", fake.URL, path), nil)
	require.NoError(t, err)
	if len(authorization) == 0 {
		request.SetBasicAuth(fakebitbucket.Email, fakebitbucket.APIToken)
	} else if authorization[0] != "" {
		request.Header.Set("Authorization", authorization[0])
	}

	return do(t, request)
}

// do sends request to the fake and returns its status and its body, decoded when it is JSON.
func do(t *testing.T, request *http.Request) answer {
	t.Helper()

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	return answer{
		status: response.StatusCode,
		body:   body,
		raw:    raw,
	}
}

func errorMessage(body map[string]any) string {
	message, _ := body["error"].(map[string]any)["message"].(string)

	return message
}

// seededWorkspace stores a workspace with slug in the fake and returns it.
func seededWorkspace(fake *fakebitbucket.Server, slug string) models.Workspace {
	return fake.SeedWorkspace(fakebitbucket.NewWorkspace(fakebitbucket.Workspace.WithSlug(slug)))
}

func TestServer_FindsAWorkspaceBySlugInAnyCaseOrByUUIDInBraces(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")

	bySlug := send(t, fake, http.MethodGet, "/2.0/workspaces/ACME")
	byUUID := send(t, fake, http.MethodGet, fmt.Sprintf("/2.0/workspaces/%s", url.PathEscape(workspace.UUID)))
	withoutBraces := send(t, fake, http.MethodGet, fmt.Sprintf("/2.0/workspaces/%s", strings.Trim(workspace.UUID, "{}")))

	assert.Equal(t, http.StatusOK, bySlug.status)
	assert.Equal(t, "acme", bySlug.body["slug"])
	assert.Equal(t, http.StatusOK, byUUID.status)
	assert.Equal(t, http.StatusNotFound, withoutBraces.status)
	assert.Equal(t, fmt.Sprintf("No workspace with identifier '%s'.", strings.Trim(workspace.UUID, "{}")), errorMessage(withoutBraces.body))
}

func TestServer_AnswersAProjectWithItsOwnerAndWorkspaceInTheShortForms(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("ABC")))

	actual := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/ABC")
	lowercase := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/abc")

	require.Equal(t, http.StatusOK, actual.status)
	embedded, _ := actual.body["workspace"].(map[string]any)
	owner, _ := actual.body["owner"].(map[string]any)
	assert.Equal(t, []string{"links", "name", "slug", "type", "uuid"}, slices.Sorted(maps.Keys(embedded)))
	// The owner is a team, which has no account ID or nickname.
	assert.Equal(t, []string{"display_name", "links", "type", "username", "uuid"}, slices.Sorted(maps.Keys(owner)))
	assert.Equal(t, http.StatusNotFound, lowercase.status, "a project key is case-sensitive")
	assert.Equal(t, recordedBody(t, "errors/project-not-found.json"), lowercase.body)
}

func TestServer_PagesTheProjectsListAsBitbucketDoes(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	for range 120 {
		fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace)))
	}

	firstPage := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects")
	lastPage := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects?pagelen=100&page=2")
	tooLarge := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects?pagelen=101")
	notANumber := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects?pagelen=many")

	assert.Equal(t, float64(50), firstPage.body["pagelen"], "50 a page when none is asked")
	assert.Equal(t, float64(120), firstPage.body["size"])
	assert.Len(t, firstPage.body["values"], 50)
	assert.Equal(t, fmt.Sprintf("%s/2.0/workspaces/acme/projects?page=2", fake.URL), firstPage.body["next"])
	assert.Len(t, lastPage.body["values"], 20)
	assert.NotContains(t, lastPage.body, "next")
	assert.Equal(t, http.StatusBadRequest, tooLarge.status)
	assert.Equal(t, recordedBody(t, "errors/invalid-pagelen.json"), tooLarge.body)
	assert.Equal(t, http.StatusBadRequest, notANumber.status)
}

func TestServer_ListsTheNewestProjectFirst(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	for _, key := range []string{"B", "C", "A"} {
		fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey(key)))
	}

	actual := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects")

	values, _ := actual.body["values"].([]any)
	keys := make([]any, 0, len(values))
	for _, value := range values {
		project, _ := value.(map[string]any)
		keys = append(keys, project["key"])
	}

	assert.Equal(t, []any{"A", "C", "B"}, keys, "the project seeded last comes first")
}

func TestServer_EmbedsTheWorkspaceAsStoredNow(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("ABC")))
	workspace.Name = "Acme renamed"
	fake.SeedWorkspace(workspace)

	actual := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/ABC")

	embedded, _ := actual.body["workspace"].(map[string]any)
	owner, _ := actual.body["owner"].(map[string]any)
	assert.Equal(t, "Acme renamed", embedded["name"], "the embedded workspace should be the stored one")
	assert.Equal(t, "Acme renamed", owner["display_name"], "the owner should be the stored workspace's team")
}

func TestServer_AnswersAnEmptyListWithAnEmptyArray(t *testing.T) {
	fake := fakebitbucket.New(t)
	seededWorkspace(fake, "acme")

	actual := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects")

	assert.Equal(t, []any{}, actual.body["values"])
}

func TestServer_ShowsAnAnonymousReaderOnlyPublicProjects(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("PRIVATE")))
	fake.SeedProject(fakebitbucket.NewProject(
		fakebitbucket.Project.WithWorkspace(workspace),
		fakebitbucket.Project.WithKey("PUBLIC"),
		fakebitbucket.Project.WithPrivate(false),
	))

	listed := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects", "")
	private := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/PRIVATE", "")
	public := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/PUBLIC", "")

	assert.Equal(t, float64(1), listed.body["size"])
	assert.Equal(t, http.StatusNotFound, private.status)
	assert.Equal(t, http.StatusOK, public.status)
}

func TestServer_HidesAProjectForTheReadsAsked(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	project := fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("NEW")))
	fake.HideProject(project, 2)

	hidden := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/NEW")
	stillHidden := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/NEW")
	shown := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/NEW")

	assert.Equal(t, []int{http.StatusNotFound, http.StatusNotFound, http.StatusOK}, []int{hidden.status, stillHidden.status, shown.status})
	assert.Equal(t, recordedBody(t, "errors/project-not-found.json"), hidden.body, "a hidden project answers the recorded 404")
}

func TestServer_HideProjectRefusesAProjectItDoesNotHave(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	notSeeded := fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("NOPE"))

	assert.PanicsWithValue(t, "fakebitbucket: HideProject: SeedProject stored no project NOPE in acme", func() {
		fake.HideProject(notSeeded, 1)
	})
}

func TestServer_AnswersAWorkspaceThatIsNotThere(t *testing.T) {
	fake := fakebitbucket.New(t)

	for _, path := range []string{"/2.0/workspaces/nobody/projects", "/2.0/workspaces/nobody/projects/ABC"} {
		actual := send(t, fake, http.MethodGet, path)

		assert.Equal(t, http.StatusNotFound, actual.status, path)
		assert.Equal(t, "No workspace with identifier 'nobody'.", errorMessage(actual.body), path)
	}
}

func TestServer_RefusesACredentialItDoesNotTake(t *testing.T) {
	fake := fakebitbucket.New(t)
	seededWorkspace(fake, "acme")

	wrongToken := send(t, fake, http.MethodGet, "/2.0/workspaces/acme", "Basic c29tZW9uZUBleGFtcGxlLmNvbTp3cm9uZw==")
	unknownAccessToken := send(t, fake, http.MethodGet, "/2.0/workspaces/acme", "Bearer not-issued")

	assert.Equal(t, http.StatusUnauthorized, wrongToken.status)
	assert.Equal(t, http.StatusUnauthorized, unknownAccessToken.status)
}

func TestServer_IssuesAccessTokensToItsConsumer(t *testing.T) {
	fake := fakebitbucket.New(t)
	seededWorkspace(fake, "acme")

	issued := requestToken(t, fake, fakebitbucket.OAuthKey, fakebitbucket.OAuthSecret, "client_credentials")
	token, _ := issued.body["access_token"].(string)
	before := send(t, fake, http.MethodGet, "/2.0/workspaces/acme", fmt.Sprintf("Bearer %s", token))
	fake.ExpireTokens()
	after := send(t, fake, http.MethodGet, "/2.0/workspaces/acme", fmt.Sprintf("Bearer %s", token))

	assert.Equal(t, http.StatusOK, issued.status)
	assert.Equal(t, float64(3600), issued.body["expires_in"])
	assert.Equal(t, http.StatusOK, before.status)
	assert.Equal(t, http.StatusUnauthorized, after.status)
}

func TestServer_RefusesATokenRequestItDoesNotTake(t *testing.T) {
	fake := fakebitbucket.New(t)

	wrongSecret := requestToken(t, fake, fakebitbucket.OAuthKey, "wrong", "client_credentials")
	otherGrant := requestToken(t, fake, fakebitbucket.OAuthKey, fakebitbucket.OAuthSecret, "authorization_code")

	assert.Equal(t, http.StatusUnauthorized, wrongSecret.status)
	assert.Equal(t, "invalid_client", wrongSecret.body["error"])
	assert.Equal(t, http.StatusBadRequest, otherGrant.status)
	assert.Equal(t, "unsupported_grant_type", otherGrant.body["error"])
}

func TestServer_FailsTheNextRequestsAndRecordsEveryRequest(t *testing.T) {
	fake := fakebitbucket.New(t)
	seededWorkspace(fake, "acme")
	fake.FailNext(http.StatusServiceUnavailable, 2)

	statuses := []int{
		send(t, fake, http.MethodGet, "/2.0/workspaces/acme").status,
		send(t, fake, http.MethodGet, "/2.0/workspaces/acme", "Bearer not-issued").status,
		send(t, fake, http.MethodGet, "/2.0/workspaces/acme?pagelen=5", "").status,
	}

	assert.Equal(t, []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusOK}, statuses)
	assert.Equal(t, []fakebitbucket.Request{
		{
			Method:     http.MethodGet,
			Path:       "/2.0/workspaces/acme",
			Query:      url.Values{},
			Credential: fakebitbucket.CredentialBasic,
			Route:      "GET /2.0/workspaces/{workspace}",
		},
		{
			Method:     http.MethodGet,
			Path:       "/2.0/workspaces/acme",
			Query:      url.Values{},
			Credential: fakebitbucket.CredentialBearer,
			Route:      "GET /2.0/workspaces/{workspace}",
		},
		{
			Method:     http.MethodGet,
			Path:       "/2.0/workspaces/acme",
			Query:      url.Values{"pagelen": {"5"}},
			Credential: fakebitbucket.CredentialNone,
			Route:      "GET /2.0/workspaces/{workspace}",
		},
	}, fake.Requests())
}

func TestServer_AnswersAPathWithoutARouteWith501(t *testing.T) {
	fake := fakebitbucket.New(t)

	actual := send(t, fake, http.MethodDelete, "/2.0/workspaces/acme")

	assert.Equal(t, http.StatusNotImplemented, actual.status)
	assert.Equal(t, "fakebitbucket has no route for DELETE /2.0/workspaces/acme", errorMessage(actual.body))
}

// recordedBody reads the body of one of the recorded answers in samples/errors.
func recordedBody(t *testing.T, name string) map[string]any {
	t.Helper()

	file, err := os.ReadFile(fmt.Sprintf("samples/%s", name))
	require.NoError(t, err)

	var sample struct {
		Body map[string]any `json:"body"`
	}
	require.NoError(t, json.Unmarshal(file, &sample))

	return sample.Body
}

func requestToken(t *testing.T, fake *fakebitbucket.Server, key, secret, grant string) answer {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, fmt.Sprintf("%s%s", fake.URL, fakebitbucket.TokenPath), strings.NewReader(url.Values{"grant_type": {grant}}.Encode()))
	require.NoError(t, err)
	request.SetBasicAuth(key, secret)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return do(t, request)
}
