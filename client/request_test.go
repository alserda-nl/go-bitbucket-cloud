package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIURL_EscapesEachSegment(t *testing.T) {
	client, err := New(nil)
	require.NoError(t, err)

	actual, err := client.apiURL([]string{"2.0", "workspaces", "{11111111-2222-4333-8444-555555555555}", "a/b"}, url.Values{"pagelen": {"100"}})
	require.NoError(t, err)

	assert.Equal(t, "https://api.bitbucket.org/2.0/workspaces/%7B11111111-2222-4333-8444-555555555555%7D/a%2Fb?pagelen=100", actual.String())
}

func TestSend_SendsTheBodyAsJSONAndDecodesTheAnswer(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusOK, nil, `{"name": "decoded"}`)}}
	client := newTestClient(t, transport)
	var answer struct {
		Name string `json:"name"`
	}

	err := client.call(t.Context(), http.MethodPut, []string{"2.0", "thing"}, nil, map[string]string{"name": "sent"}, &answer)
	require.NoError(t, err)

	assert.Equal(t, "decoded", answer.Name)
	assert.Equal(t, []string{`{"name":"sent"}`}, transport.bodies)
	assert.Equal(t, "application/json", transport.requests[0].Header.Get("Content-Type"))
	assert.Equal(t, "application/json", transport.requests[0].Header.Get("Accept"))
}

func TestSend_IgnoresTheAnswerWithoutAPlaceForIt(t *testing.T) {
	client := newTestClient(t, &recordingTransport{responses: []*http.Response{stubResponse(http.StatusNoContent, nil, "")}})

	err := client.call(t.Context(), http.MethodDelete, []string{"2.0", "thing"}, nil, nil, nil)

	assert.NoError(t, err)
}

func TestSend_Fails(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		body      any
		transport http.RoundTripper
		expected  string
	}{
		{
			name:      "a body that is not JSON",
			method:    http.MethodPost,
			body:      make(chan int),
			transport: &recordingTransport{},
			expected:  "bitbucket: POST /2.0/thing: encoding the body: json: unsupported type: chan int",
		},
		{
			name:      "a method that is not one",
			method:    "NOT A METHOD",
			transport: &recordingTransport{},
			expected:  `bitbucket: NOT A METHOD /2.0/thing: net/http: invalid method "NOT A METHOD"`,
		},
		{
			name:   "a connection that fails, without the address and its query",
			method: http.MethodGet,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}),
			expected: "bitbucket: GET /2.0/thing: connection refused",
		},
		{
			name:   "an answer that breaks off",
			method: http.MethodGet,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{},
					Body:       failingBody{},
				}, nil
			}),
			expected: "bitbucket: GET /2.0/thing: reading the 200 answer: connection reset by peer",
		},
		{
			name:      "an answer that is not JSON",
			method:    http.MethodGet,
			transport: &recordingTransport{responses: []*http.Response{stubResponse(http.StatusOK, http.Header{"Content-Type": {"text/html"}}, "<html>")}},
			expected:  "bitbucket: GET /2.0/thing: decoding the 200 answer (text/html): invalid character '<' looking for beginning of value",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newTestClient(t, testCase.transport)

			var answer map[string]any
			err := client.call(t.Context(), testCase.method, []string{"2.0", "thing"}, url.Values{"signed": {"secret"}}, testCase.body, &answer)

			assert.EqualError(t, err, testCase.expected, "call failed with the wrong error for %s", testCase.name)
		})
	}
}

func TestSend_TurnsAnErrorStatusIntoAnAPIError(t *testing.T) {
	header := http.Header{"Content-Type": {"application/json"}}
	client := newTestClient(t, &recordingTransport{responses: []*http.Response{stubResponse(http.StatusNotFound, header, `{"type": "error", "error": {"message": "gone"}}`)}})

	err := client.call(t.Context(), http.MethodGet, []string{"2.0", "thing"}, nil, nil, nil)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "gone", apiErr.Message)
	assert.ErrorIs(t, err, ErrNotFound)
}

// newTestClient makes a client whose requests go to transport, without retry waits.
func newTestClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()

	client, err := New(nil, WithHTTPClient(&http.Client{Transport: transport}), WithRetryWait(0))
	require.NoError(t, err)

	return client
}

func TestCall_RefusesAPathPartThatWouldAskForAnotherPath(t *testing.T) {
	cases := []struct {
		name    string
		segment string
	}{
		{
			name:    "an empty part",
			segment: "",
		},
		{
			name:    "a dot",
			segment: ".",
		},
		{
			name:    "two dots",
			segment: "..",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &recordingTransport{}
			client := newTestClient(t, transport)

			err := client.call(t.Context(), http.MethodGet, []string{"2.0", "workspaces", "acme", "projects", testCase.segment}, nil, nil, nil)

			assert.EqualError(t, err, fmt.Sprintf(`bitbucket: invalid option: the path /2.0/workspaces/acme/projects/%s has a part that is empty, "." or ".."`, testCase.segment), "call should refuse %s", testCase.name)
			assert.ErrorIs(t, err, ErrInvalidOption, "call should refuse %s as an invalid option", testCase.name)
			assert.Empty(t, transport.requests, "call should send no request for %s", testCase.name)
		})
	}
}

func TestSend_SaysHowOftenAFailedCallWasSent(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{stubResponse(http.StatusServiceUnavailable, nil, "")}}
	client := newTestClient(t, transport)

	err := client.call(t.Context(), http.MethodGet, []string{"2.0", "thing"}, nil, nil, nil)

	assert.EqualError(t, err, "bitbucket: GET /2.0/thing: 503 Service Unavailable (after 4 attempts)")
}

// A transport of the caller's may leave an answer's request out; the error then names no attempts.
func TestSend_TakesAnAnswerWithoutItsRequest(t *testing.T) {
	client := newTestClient(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return stubResponse(http.StatusNotFound, nil, ""), nil
	}))

	err := client.call(t.Context(), http.MethodGet, []string{"2.0", "thing"}, nil, nil, nil)

	assert.EqualError(t, err, "bitbucket: GET /2.0/thing: 404 Not Found")
}
