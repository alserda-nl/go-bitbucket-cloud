package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIError_ReadsEveryShapeBitbucketAnswers(t *testing.T) {
	cases := []struct {
		name     string
		sample   string
		expected APIError
	}{
		{
			name:   "a repository endpoint's 404",
			sample: "errors/project-not-found.json",
			expected: APIError{
				Message: "You may not have access to this project or it no longer exists in this workspace. If you think this project exists and you have access, make sure you are authenticated.",
			},
		},
		{
			name:   "a 403 for a missing scope",
			sample: "errors/scope-refused.json",
			expected: APIError{
				Message:        "Your credentials lack one or more required privilege scopes.",
				RequiredScopes: []string{"read:project:bitbucket"},
				GrantedScopes:  []string{"admin:project:bitbucket"},
			},
		},
		{
			name:   "a detail in text",
			sample: "errors/project-not-empty.json",
			expected: APIError{
				Message: "Non-empty project deletion forbidden",
				Detail:  "Please make sure that the project is empty before deleting it.",
			},
		},
		{
			name:   "a field with one complaint",
			sample: "errors/commit-not-found.json",
			expected: APIError{
				Message: "Bad request",
				Fields:  map[string][]string{"target.hash": {"Commit not found: main"}},
			},
		},
		{
			name:   "a field with a list of complaints",
			sample: "errors/parent-not-found.json",
			expected: APIError{
				Message: "parents: Commit not found: 0000000000000000000000000000000000000000",
				Fields:  map[string][]string{"parents": {"Commit not found: 0000000000000000000000000000000000000000"}},
			},
		},
		{
			name:   "a branch error's key, which its code repeats",
			sample: "errors/branch-exists.json",
			expected: APIError{
				Message: `Branch "develop" already exists`,
				Key:     "BRANCH_ALREADY_EXISTS",
			},
		},
		{
			name:   "a 500 with an id",
			sample: "errors/server-error.json",
			expected: APIError{
				Message: "Something went wrong",
				ID:      "00000000000000000000000000000000",
			},
		},
		{
			name:   "the Pipelines services' key and arguments",
			sample: "errors/variable-duplicate.json",
			expected: APIError{
				Message:   "Conflict",
				Detail:    "A variable with the key provided already exists for account {00000000-0000-4000-8000-000000000001}, environment {00000000-0000-4000-8000-000000000002}.",
				Key:       "variable-service.variable.duplicate",
				Arguments: map[string]any{"externalId": "{00000000-0000-4000-8000-000000000001}"},
			},
		},
		{
			name:   "a runner's key, message and arguments without an error object",
			sample: "errors/runner-not-found.json",
			expected: APIError{
				Message:   "Runner with the uuid {00000000-0000-4000-8000-000000000001} not found.",
				Key:       "agent-service.runner.not-found",
				Arguments: map[string]any{},
			},
		},
		{
			name:   "a Pipelines 500, whose code repeats its status",
			sample: "errors/schedule-server-error.json",
			expected: APIError{
				Message: "There was an error processing your request. It has been logged (ID 0000000000000000).",
			},
		},
		{
			name:     "a 404 without a body",
			sample:   "errors/file-not-found.json",
			expected: APIError{},
		},
		{
			name:   "an answer in text",
			sample: "errors/text-answer.json",
			expected: APIError{
				Text: "Bad request",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, contentType, body := readAnswerSample(t, testCase.sample)
			expected := testCase.expected
			expected.Method = http.MethodGet
			expected.Path = "/2.0/a/path"
			expected.StatusCode = status

			actual := newAPIError(http.MethodGet, "/2.0/a/path", status, contentType, body)

			assert.Equal(t, &expected, actual, "newAPIError(%s) read the answer wrong", testCase.sample)
		})
	}
}

func TestNewAPIError_ReadsARemoved10PathsAnswer(t *testing.T) {
	// Written by hand in the shape Bitbucket answers a removed 1.0 path with.
	body := []byte(`{"message": "This API is no longer supported.", "status": 410}`)

	actual := newAPIError(http.MethodGet, "/1.0/privileges/a/b", http.StatusGone, "application/json", body)

	assert.Equal(t, "This API is no longer supported.", actual.Message)
	assert.Equal(t, "bitbucket: GET /1.0/privileges/a/b: 410 Gone: This API is no longer supported.", actual.Error())
}

func TestNewAPIError_ReadsAnOAuthRefusal(t *testing.T) {
	body := []byte(`{"error": "invalid_client", "error_description": "Invalid OAuth client credentials"}`)

	actual := newAPIError(http.MethodPost, "/site/oauth2/access_token", http.StatusUnauthorized, "application/json", body)

	assert.Equal(t, "invalid_client: Invalid OAuth client credentials", actual.Message)
}

func TestNewAPIError_KeepsTheTextOfJSONInAnUnknownShape(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "a list",
			body: `["not", "an", "object"]`,
		},
		{
			name: "an object without a field of a known shape",
			body: `{"detail": "x"}`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := newAPIError(http.MethodGet, "/2.0/a/path", http.StatusBadRequest, "application/json", []byte(testCase.body))

			assert.Equal(t, testCase.body, actual.Text, "newAPIError(%s) should keep the text", testCase.body)
		})
	}
}

func TestAPIError_ErrorSaysTheCallTheStatusAndBitbucketsWords(t *testing.T) {
	cases := []struct {
		name     string
		apiErr   APIError
		expected string
	}{
		{
			name: "a scope refusal names the scopes the credential misses",
			apiErr: APIError{
				Method:         http.MethodGet,
				Path:           "/2.0/workspaces/acme/projects/ABC",
				StatusCode:     http.StatusForbidden,
				Message:        "Your credentials lack one or more required privilege scopes.",
				RequiredScopes: []string{"read:project:bitbucket", "read:repository:bitbucket"},
				GrantedScopes:  []string{"read:repository:bitbucket"},
			},
			expected: "bitbucket: GET /2.0/workspaces/acme/projects/ABC: 403 Forbidden: Your credentials lack one or more required privilege scopes. (the credential misses the scopes read:project:bitbucket)",
		},
		{
			name: "detail, fields and key follow the message",
			apiErr: APIError{
				Method:     http.MethodPost,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusConflict,
				Message:    "Conflict",
				Detail:     "It already exists.",
				Fields: map[string][]string{
					"name": {"too long"},
					"key":  {"taken", "lowercase"},
				},
				Key: "variable-service.variable.duplicate",
			},
			expected: "bitbucket: POST /2.0/a/path: 409 Conflict: Conflict: It already exists. (key: taken, lowercase; name: too long) (variable-service.variable.duplicate)",
		},
		{
			name: "a long text is cut",
			apiErr: APIError{
				Method:     http.MethodGet,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusPaymentRequired,
				Text:       strings.Repeat("x", 301),
			},
			expected: fmt.Sprintf("bitbucket: GET /2.0/a/path: 402 Payment Required: %s...", strings.Repeat("x", 300)),
		},
		{
			name: "a long text is cut between characters, not bytes",
			apiErr: APIError{
				Method:     http.MethodGet,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusPaymentRequired,
				Text:       strings.Repeat("é", 301),
			},
			expected: fmt.Sprintf("bitbucket: GET /2.0/a/path: 402 Payment Required: %s...", strings.Repeat("é", 300)),
		},
		{
			name: "a text of 300 characters is kept whole",
			apiErr: APIError{
				Method:     http.MethodGet,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusPaymentRequired,
				Text:       strings.Repeat("x", 300),
			},
			expected: fmt.Sprintf("bitbucket: GET /2.0/a/path: 402 Payment Required: %s", strings.Repeat("x", 300)),
		},
		{
			name: "a 500 names its error ID, for Atlassian's support",
			apiErr: APIError{
				Method:     http.MethodGet,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusInternalServerError,
				Message:    "Something went wrong",
				ID:         "0123456789abcdef0123456789abcdef",
			},
			expected: "bitbucket: GET /2.0/a/path: 500 Internal Server Error: Something went wrong (error ID 0123456789abcdef0123456789abcdef)",
		},
		{
			name: "a call sent more than once says how often",
			apiErr: APIError{
				Method:     http.MethodGet,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusServiceUnavailable,
				Attempts:   4,
			},
			expected: "bitbucket: GET /2.0/a/path: 503 Service Unavailable (after 4 attempts)",
		},
		{
			name: "nothing but the status",
			apiErr: APIError{
				Method:     http.MethodGet,
				Path:       "/2.0/a/path",
				StatusCode: http.StatusNotFound,
			},
			expected: "bitbucket: GET /2.0/a/path: 404 Not Found",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := testCase.apiErr.Error()

			assert.Equal(t, testCase.expected, actual, "APIError.Error() returned the wrong text (%s)", testCase.name)
		})
	}
}

func TestAPIError_EveryNotFoundMatchesErrNotFound(t *testing.T) {
	notFound := &APIError{StatusCode: http.StatusNotFound}
	// Stopping a pipeline run that does not exist answers 400 "Unexpected response body".
	badRequest := &APIError{StatusCode: http.StatusBadRequest}

	assert.ErrorIs(t, notFound, ErrNotFound)
	assert.NotErrorIs(t, notFound, ErrLogin, "a 404 must match ErrNotFound only")
	assert.NotErrorIs(t, notFound, ErrInvalidOption, "a 404 must match ErrNotFound only")
	assert.False(t, errors.Is(badRequest, ErrNotFound), "a 400 must not match ErrNotFound")
}

// readAnswerSample reads one of the fake's answer samples: a real answer's status, content type
// and body, with names and IDs replaced.
func readAnswerSample(t *testing.T, name string) (int, string, []byte) {
	t.Helper()

	file, err := os.ReadFile(fmt.Sprintf("../fakebitbucket/samples/%s", name))
	require.NoError(t, err)

	var sample struct {
		Status      int             `json:"status"`
		ContentType string          `json:"content_type"`
		Body        json.RawMessage `json:"body"`
	}
	require.NoError(t, json.Unmarshal(file, &sample))

	var text string
	switch {
	case string(sample.Body) == "null":
		return sample.Status, sample.ContentType, nil
	case json.Unmarshal(sample.Body, &text) == nil:
		return sample.Status, sample.ContentType, []byte(text)
	default:
		return sample.Status, sample.ContentType, sample.Body
	}
}

func TestWithoutURL_KeepsAnErrorWithoutAnAddress(t *testing.T) {
	plain := errors.New("connection refused")

	assert.Equal(t, plain, withoutURL(plain))
	assert.Equal(t, plain, withoutURL(&url.Error{
		Op:  "Get",
		URL: "https://x/?signature=secret",
		Err: plain,
	}))
}

func TestWithoutURL_DropsEveryAddressInTheChain(t *testing.T) {
	cause := errors.New("connection reset")
	nested := &url.Error{
		Op:  "Get",
		URL: "https://x/?signature=outer",
		Err: fmt.Errorf("proxy: %w", &url.Error{
			Op:  "Get",
			URL: "https://y/?signature=inner",
			Err: cause,
		}),
	}

	actual := withoutURL(nested)

	assert.Equal(t, cause, actual)
	assert.NotContains(t, actual.Error(), "signature")
}
