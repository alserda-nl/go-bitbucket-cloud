package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const (
	// jsonMediaType is the media type of the bodies the API takes and gives.
	jsonMediaType = "application/json"
	// textShown is how much of an answer that is not JSON an error's text shows: a 402 on
	// downloads was a whole HTML page (2026-10-26).
	textShown = 300
)

// Sentinel errors, matched with errors.Is.
var (
	// ErrInvalidConfig is returned by New for a credential that misses a part or an option out of range.
	ErrInvalidConfig = errors.New("bitbucket: invalid client configuration")
	// ErrInvalidOption is returned by a call given an option out of range, before any request.
	ErrInvalidOption = errors.New("bitbucket: invalid option")
	// ErrNotFound matches every 404, whatever its shape. A "not found" that Bitbucket answers with
	// 400, such as stopping a pipeline run that does not exist (2026-10-26), does not match it.
	ErrNotFound = errors.New("bitbucket: not found")
	// ErrLogin is returned when the OAuth consumer gets no access token. It never matches
	// ErrNotFound: a 404 from the token address says nothing about the object a call asked for.
	// Its text has no "bitbucket:" of its own, as the call's error around it starts with one.
	ErrLogin = errors.New("logging in with the OAuth consumer failed")
)

// APIError is Bitbucket's answer to a call when its status is 400 or above. Its error answers
// differ with the service behind the endpoint; APIError keeps what a caller can use from each.
type APIError struct {
	// Method and Path are the call's, the path without its query.
	Method string
	Path   string
	// StatusCode is the HTTP status.
	StatusCode int
	Message    string
	// Detail is the detail Bitbucket gave as text.
	Detail string
	// Fields names the fields a 400 refused, with what is wrong with each.
	Fields map[string][]string
	// Key names the error in the Pipelines, deployment and runner services:
	// "variable-service.variable.duplicate", "agent-service.runner.not-found".
	Key       string
	Arguments map[string]any
	// ID identifies a 500 for Atlassian's support.
	ID string
	// Attempts is how often the call was sent, retries included.
	Attempts int
	// RequiredScopes and GrantedScopes come with a 403 for a missing scope: the scopes the call
	// needs and the ones the credential has.
	RequiredScopes []string
	GrantedScopes  []string
	// Text is the answer when it is not JSON, such as a text/plain 400 or a text/html 402.
	Text string
}

// Error returns the call, the status, Bitbucket's words, on a 403 for a scope the scopes the
// credential misses, a 500's error ID, and how often the call was sent when it was more than once.
func (e *APIError) Error() string {
	return fmt.Sprintf("bitbucket: %s", e.describe())
}

// describe says what Error says, without the "bitbucket:" that starts every error of this package.
func (e *APIError) describe() string {
	parts := []string{fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))}
	for _, part := range []string{e.Message, e.Detail, shorten(e.Text)} {
		if part != "" {
			parts = append(parts, part)
		}
	}

	text := strings.Join(parts, ": ")
	if len(e.Fields) > 0 {
		text = fmt.Sprintf("%s (%s)", text, describeFields(e.Fields))
	}
	if e.Key != "" {
		text = fmt.Sprintf("%s (%s)", text, e.Key)
	}
	if missing := e.MissingScopes(); len(missing) > 0 {
		text = fmt.Sprintf("%s (the credential misses the scopes %s)", text, strings.Join(missing, ", "))
	}
	if e.ID != "" {
		text = fmt.Sprintf("%s (error ID %s)", text, e.ID)
	}
	if e.Attempts > 1 {
		text = fmt.Sprintf("%s (after %d attempts)", text, e.Attempts)
	}

	return text
}

// Is makes errors.Is(err, ErrNotFound) true for every 404.
func (e *APIError) Is(target error) bool {
	return target == ErrNotFound && e.StatusCode == http.StatusNotFound
}

// MissingScopes returns the scopes a 403 required that the credential was not granted.
func (e *APIError) MissingScopes() []string {
	var missing []string
	for _, scope := range e.RequiredScopes {
		if !slices.Contains(e.GrantedScopes, scope) {
			missing = append(missing, scope)
		}
	}

	return missing
}

// errorAnswer holds every top-level field of the error answers recorded from 2026-10-12 to
// 2026-10-30; the fields an answer lacks stay empty. As JSON:
//   - {"type": "error", "error": {"message", "detail", "fields", "id", "data"}}: the repository
//     endpoints, a branch error and the Pipelines services, whose data holds their key and arguments;
//   - {"key", "message", "arguments"}: the runners;
//   - {"code", "message"}: a Pipelines 500, whose code repeats its status;
//   - {"error", "error_description"}: the token address, as RFC 6749, section 5.2, has it.
//
// An answer in text, such as a text/html 402, or without a body keeps its text instead.
type errorAnswer struct {
	// Error is an object in the repository endpoints' and the Pipelines services' shapes, and
	// text in an OAuth refusal (RFC 6749, section 5.2).
	Error            json.RawMessage `json:"error"`
	ErrorDescription string          `json:"error_description"`
	Key              string          `json:"key"`
	Message          string          `json:"message"`
	Arguments        map[string]any  `json:"arguments"`
}

// known reports whether the answer has a field of one of the six shapes.
func (a errorAnswer) known() bool {
	return a.Error != nil || a.Message != "" || a.Key != ""
}

// errorObject is the "error" object of the repository endpoints and of the Pipelines services.
type errorObject struct {
	Message string `json:"message"`
	// Detail is text, or the required and granted scopes on a 403 for a scope.
	Detail json.RawMessage `json:"detail"`
	// Fields maps a field to one complaint or to a list of them.
	Fields map[string]json.RawMessage `json:"fields"`
	ID     string                     `json:"id"`
	// Data holds a branch error's key, or the Pipelines services' key and arguments.
	Data map[string]any `json:"data"`
}

// newAPIError reads an answer of 400 or above into an APIError, whatever its shape. An answer
// that is not JSON, or JSON in a shape not known here, keeps its text.
func newAPIError(method, path string, status int, contentType string, body []byte) *APIError {
	apiErr := &APIError{
		Method:     method,
		Path:       path,
		StatusCode: status,
	}

	var answer errorAnswer
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType != jsonMediaType || json.Unmarshal(body, &answer) != nil || !answer.known() {
		apiErr.Text = strings.TrimSpace(string(body))
		return apiErr
	}

	apiErr.Message = answer.Message
	apiErr.Key = answer.Key
	apiErr.Arguments = answer.Arguments

	var object errorObject
	if err := json.Unmarshal(answer.Error, &object); err == nil {
		readErrorObject(apiErr, object)
	} else if description := rawText(answer.Error); description != "" {
		// An OAuth refusal: {"error": "invalid_client", "error_description": "..."}.
		apiErr.Message = strings.TrimSuffix(fmt.Sprintf("%s: %s", description, answer.ErrorDescription), ": ")
	}

	return apiErr
}

// readErrorObject copies the "error" object's fields into apiErr.
func readErrorObject(apiErr *APIError, object errorObject) {
	apiErr.Message = object.Message
	apiErr.ID = object.ID
	if object.Data != nil {
		apiErr.Key, _ = object.Data["key"].(string)
		apiErr.Arguments, _ = object.Data["arguments"].(map[string]any)
	}

	var scopes struct {
		Required []string `json:"required"`
		Granted  []string `json:"granted"`
	}
	if err := json.Unmarshal(object.Detail, &scopes); err == nil {
		apiErr.RequiredScopes = scopes.Required
		apiErr.GrantedScopes = scopes.Granted
	} else {
		apiErr.Detail = rawText(object.Detail)
	}

	for field, complaint := range object.Fields {
		if apiErr.Fields == nil {
			apiErr.Fields = map[string][]string{}
		}

		var complaints []string
		if json.Unmarshal(complaint, &complaints) != nil {
			complaints = []string{rawText(complaint)}
		}
		apiErr.Fields[field] = complaints
	}
}

// rawText returns a JSON string's text, another JSON value's literal, or nothing for null or no value.
func rawText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	return string(raw)
}

// describeFields lists refused fields in a stable order: "a: x; b: y, z".
func describeFields(fields map[string][]string) string {
	described := make([]string, 0, len(fields))
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		described = append(described, fmt.Sprintf("%s: %s", name, strings.Join(fields[name], ", ")))
	}

	return strings.Join(described, "; ")
}

// shorten cuts text to textShown characters.
func shorten(text string) string {
	runes := []rune(text)
	if len(runes) <= textShown {
		return text
	}

	return fmt.Sprintf("%s...", string(runes[:textShown]))
}

// withoutURL drops the address from an *url.Error: its query can carry a signed token, as a
// pipeline cache's download address did (2026-10-26).
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}

	return err
}
