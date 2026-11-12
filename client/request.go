package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// apiVersion is the version of Bitbucket's REST API the calls go to.
const apiVersion = "2.0"

// apiPath returns the segments of a path in Bitbucket's 2.0 API:
// apiPath("workspaces", "acme") returns ["2.0", "workspaces", "acme"].
func apiPath(segments ...string) []string {
	return append([]string{apiVersion}, segments...)
}

// apiURL returns the address of a call on the client's endpoint, each segment escaped:
// ["2.0", "workspaces", "{abc}"] gives https://api.bitbucket.org/2.0/workspaces/%7Babc%7D. An
// empty segment, "." or ".." would make the call ask for another path, so it fails with
// ErrInvalidOption.
func (c *Client) apiURL(segments []string, query url.Values) (*url.URL, error) {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("%w: the path /%s has a part that is empty, \".\" or \"..\"", ErrInvalidOption, strings.Join(segments, "/"))
		}

		escaped[i] = url.PathEscape(segment)
	}

	address := *c.endpoint
	address.Path = strings.Join(append([]string{c.endpoint.Path}, segments...), "/")
	address.RawPath = strings.Join(append([]string{c.endpoint.EscapedPath()}, escaped...), "/")
	address.RawQuery = query.Encode()

	return &address, nil
}

// callFor sends a call to the API path made of segments, with body as JSON unless it is nil, and
// returns the answer decoded as a T.
func callFor[T any](ctx context.Context, c *Client, method string, segments []string, body any) (*T, error) {
	var answer T
	if err := c.call(ctx, method, segments, nil, body, &answer); err != nil {
		return nil, err
	}

	return &answer, nil
}

// call sends a request to the API path made of segments and decodes the answer into out.
func (c *Client) call(ctx context.Context, method string, segments []string, query url.Values, body, out any) error {
	address, err := c.apiURL(segments, query)
	if err != nil {
		return err
	}

	return c.send(ctx, method, address, body, out)
}

// send sends one request with body as JSON, when there is one, and decodes the answer into out,
// when out is not nil. An answer of 400 or above becomes an *APIError, and one longer than
// answerLimit fails.
func (c *Client) send(ctx context.Context, method string, address *url.URL, body, out any) error {
	path := address.EscapedPath()

	var content io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("bitbucket: %s %s: encoding the body: %w", method, path, err)
		}

		// A bytes.Reader lets the retry and the OAuth login send the body again.
		content = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, address.String(), content)
	if err != nil {
		return fmt.Errorf("bitbucket: %s %s: %w", method, path, err)
	}

	request.Header.Set("Accept", jsonMediaType)
	if body != nil {
		request.Header.Set("Content-Type", jsonMediaType)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("bitbucket: %s %s: %w", method, path, withoutURL(err))
	}
	defer func() { _ = response.Body.Close() }()

	answer, err := readAnswer(response.Body)
	if err != nil {
		return fmt.Errorf("bitbucket: %s %s: reading %s: %w", method, path, describeAnswer(response), err)
	}
	if response.StatusCode >= http.StatusBadRequest {
		apiErr := newAPIError(method, path, response.StatusCode, response.Header.Get("Content-Type"), answer)
		apiErr.Attempts = attemptOf(response.Request)
		return apiErr
	}
	if out == nil {
		return nil
	}

	if err := json.Unmarshal(answer, out); err != nil {
		return fmt.Errorf("bitbucket: %s %s: decoding %s: %w", method, path, describeAnswer(response), err)
	}

	return nil
}

// answerLimit is the most of an answer send and the token request read, so an answer that does
// not end cannot fill the memory.
const answerLimit = 32 << 20

// readAnswer reads body whole, up to answerLimit, and fails for a longer one.
func readAnswer(body io.Reader) ([]byte, error) {
	answer, err := io.ReadAll(io.LimitReader(body, answerLimit+1))
	if err != nil {
		return nil, err
	}
	if len(answer) > answerLimit {
		return nil, fmt.Errorf("it is longer than %d MiB, the most the client reads", answerLimit>>20)
	}

	return answer, nil
}

// describeAnswer names an answer by its status and its content type, if any, never by its body:
// "the 200 answer (text/html)".
func describeAnswer(response *http.Response) string {
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		return fmt.Sprintf("the %d answer (%s)", response.StatusCode, contentType)
	}

	return fmt.Sprintf("the %d answer", response.StatusCode)
}

// attemptOf returns the number the retries gave req's attempt, or 0 for a nil req or one the
// retries did not send.
func attemptOf(req *http.Request) int {
	if req == nil {
		return 0
	}

	attempt, _ := req.Context().Value(attemptKey{}).(int)

	return attempt
}
