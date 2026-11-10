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

// answer is what send, sendAnonymously and do return: the fake's answer to one request.
type answer struct {
	// status is the answer's HTTP status.
	status int
	// body is the answer decoded when it is a JSON object.
	body map[string]any
	// raw is the answer as it came.
	raw []byte
}

// send sends a request to the fake as the API token's holder, or with the given Authorization.
func send(t *testing.T, fake *fakebitbucket.Server, method, path string, authorization ...string) answer {
	t.Helper()

	request := newRequest(t, fake, method, path)
	if len(authorization) == 0 {
		request.SetBasicAuth(fakebitbucket.Email, fakebitbucket.APIToken)
	} else {
		request.Header.Set("Authorization", authorization[0])
	}

	return do(t, request)
}

// sendAnonymously sends a request to the fake without a credential.
func sendAnonymously(t *testing.T, fake *fakebitbucket.Server, method, path string) answer {
	t.Helper()

	return do(t, newRequest(t, fake, method, path))
}

// newRequest returns a request to path on the fake, without a body or a credential.
func newRequest(t *testing.T, fake *fakebitbucket.Server, method, path string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, fmt.Sprintf("%s%s", fake.URL, path), nil)
	require.NoError(t, err)

	return request
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

func TestServer_AnswersAPagePastTheLastWithAnEmptyPage(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	for range 3 {
		fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace)))
	}

	afterTheLast := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects?page=2")
	// Its first item, at (page - 1) * 50, lies past the largest int.
	huge := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects?page=184467440737095518")

	assert.Equal(t, []any{}, afterTheLast.body["values"], "page 2 of 3 projects in pages of 50")
	assert.NotContains(t, afterTheLast.body, "next", "page 2 of 3 projects in pages of 50")

	assert.Equal(t, http.StatusOK, huge.status, "page 184467440737095518")
	assert.Equal(t, []any{}, huge.body["values"], "page 184467440737095518")
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

	listed := sendAnonymously(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects")
	private := sendAnonymously(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/PRIVATE")
	public := sendAnonymously(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/PUBLIC")

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

func TestServer_LeavesAHiddenProjectOutOfTheListWithoutSpendingARead(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	project := fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("NEW")))
	fake.HideProject(project, 1)

	listedWhileHidden := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects")
	read := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects/NEW")
	listedAfterTheRead := send(t, fake, http.MethodGet, "/2.0/workspaces/acme/projects")

	assert.Equal(t, float64(0), listedWhileHidden.body["size"], "the list should leave out a hidden project")
	assert.Equal(t, http.StatusNotFound, read.status, "the list should leave the hidden project's read unspent")
	assert.Equal(t, float64(1), listedAfterTheRead.body["size"], "the list should show the project once its reads are spent")
}

func TestServer_HideProjectRefusesAProjectItDoesNotHave(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	notSeeded := fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("NOPE"))

	assert.PanicsWithValue(t, "fakebitbucket: HideProject: SeedProject stored no project NOPE in acme", func() {
		fake.HideProject(notSeeded, 1)
	})
}

func TestServer_SeedProjectRefusesAProjectWhoseWorkspaceItDoesNotHave(t *testing.T) {
	fake := fakebitbucket.New(t)
	elsewhere := fakebitbucket.NewWorkspace(fakebitbucket.Workspace.WithSlug("elsewhere"))
	project := fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(elsewhere), fakebitbucket.Project.WithKey("ABC"))

	assert.PanicsWithValue(t, "fakebitbucket: SeedProject: SeedWorkspace stored no workspace elsewhere", func() {
		fake.SeedProject(project)
	})
}

// TestServer_AnswersEveryFieldOfTheRecordedAnswers compares each answer's fields and their JSON
// types with the sample it starts from, one of Bitbucket's own answers, so a field the fake drops,
// adds or retypes fails here; only a null in the sample matches any type.
func TestServer_AnswersEveryFieldOfTheRecordedAnswers(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := seededWorkspace(fake, "acme")
	fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("ABC")))

	cases := []struct {
		name   string
		path   string
		sample string
	}{
		{
			name:   "a workspace",
			path:   "/2.0/workspaces/acme",
			sample: "workspace.json",
		},
		{
			name:   "a project",
			path:   "/2.0/workspaces/acme/projects/ABC",
			sample: "project.json",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := send(t, fake, http.MethodGet, testCase.path)
			require.Equal(t, http.StatusOK, actual.status)

			var expected map[string]any
			require.NoError(t, json.Unmarshal(readSample(t, testCase.sample), &expected))

			assert.Empty(t, differentFields("", expected, actual.body), "GET %s should answer the fields of %s, each with its JSON type", testCase.path, testCase.sample)
		})
	}
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
		sendAnonymously(t, fake, http.MethodGet, "/2.0/workspaces/acme?pagelen=5").status,
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

func TestServer_AnswersTheFailuresInTheOrderAsked(t *testing.T) {
	fake := fakebitbucket.New(t)
	seededWorkspace(fake, "acme")
	fake.FailNext(http.StatusTooManyRequests, 1)
	fake.FailNext(http.StatusServiceUnavailable, 1)

	statuses := []int{
		send(t, fake, http.MethodGet, "/2.0/workspaces/acme").status,
		send(t, fake, http.MethodGet, "/2.0/workspaces/acme").status,
		send(t, fake, http.MethodGet, "/2.0/workspaces/acme").status,
	}

	assert.Equal(t, []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusOK}, statuses, "FailNext(429, 1), then FailNext(503, 1)")
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

	var sample struct {
		Body map[string]any `json:"body"`
	}
	require.NoError(t, json.Unmarshal(readSample(t, name), &sample))

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

// readSample reads one of the samples: Bitbucket's answers with names and IDs replaced.
func readSample(t *testing.T, name string) []byte {
	t.Helper()

	sample, err := os.ReadFile(fmt.Sprintf("samples/%s", name))
	require.NoError(t, err)

	return sample
}

// differentFields lists every field that expected or actual has and the other lacks, and every
// field the two hold as different JSON types; only a null in expected, the sample, matches any.
func differentFields(path string, expected, actual any) []string {
	expectedObject, expectedIsObject := expected.(map[string]any)
	actualObject, actualIsObject := actual.(map[string]any)

	switch {
	case expected == nil:
		return nil
	case actual == nil:
		return []string{fmt.Sprintf("%s: want %T, got null", path, expected)}
	case expectedIsObject && actualIsObject:
		var different []string
		for _, key := range slices.Sorted(maps.Keys(expectedObject)) {
			if _, found := actualObject[key]; !found {
				different = append(different, fmt.Sprintf("%s.%s: missing", path, key))
				continue
			}

			different = append(different, differentFields(fmt.Sprintf("%s.%s", path, key), expectedObject[key], actualObject[key])...)
		}

		for _, key := range slices.Sorted(maps.Keys(actualObject)) {
			if _, found := expectedObject[key]; !found {
				different = append(different, fmt.Sprintf("%s.%s: not in the sample", path, key))
			}
		}

		return different
	case fmt.Sprintf("%T", expected) != fmt.Sprintf("%T", actual):
		return []string{fmt.Sprintf("%s: want %T, got %T", path, expected, actual)}
	default:
		return nil
	}
}
