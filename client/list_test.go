package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type item struct {
	Name string `json:"name"`
}

func TestList_FollowsNextLinks(t *testing.T) {
	cases := []struct {
		name            string
		next            string
		expectedAddress string
	}{
		{
			name:            "a link into the 2.0 API on the endpoint, as given",
			next:            "https://api.bitbucket.org/2.0/repositories/%7Bw%7D/%7Br%7D/deployments?page=2&pagelen=100",
			expectedAddress: "https://api.bitbucket.org/2.0/repositories/%7Bw%7D/%7Br%7D/deployments?page=2&pagelen=100",
		},
		{
			name:            "a relative link outside the 2.0 API, through the list's own path",
			next:            "/rest/1.0/accounts/%7Bw%7D/repositories/%7Br%7D/pipelines_config/schedules/%7Bs%7D/executions?page=2&pagelen=10",
			expectedAddress: "https://api.bitbucket.org/2.0/things?page=2&pagelen=100",
		},
		{
			name:            "a link to the endpoint's host over http, through the list's own path",
			next:            "http://api.bitbucket.org/2.0/things?page=2",
			expectedAddress: "https://api.bitbucket.org/2.0/things?page=2&pagelen=100",
		},
		{
			name:            "a link to another host, through the list's own path",
			next:            "https://elsewhere.example.com/2.0/things?page=3",
			expectedAddress: "https://api.bitbucket.org/2.0/things?page=3&pagelen=100",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &recordingTransport{responses: []*http.Response{
				stubResponse(http.StatusOK, nil, fmt.Sprintf(`{"values": [{"name": "a"}], "next": %q}`, testCase.next)),
				stubResponse(http.StatusOK, nil, `{"values": [{"name": "b"}]}`),
			}}
			client := newTestClient(t, transport)

			items, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, nil)
			require.NoError(t, err)

			assert.Equal(t, []item{{Name: "a"}, {Name: "b"}}, items, "list returned the wrong items after the next link %q", testCase.next)
			assert.Equal(t, "https://api.bitbucket.org/2.0/things?pagelen=100", transport.requests[0].URL.String(), "list asked for the wrong first page")
			assert.Equal(t, testCase.expectedAddress, transport.requests[1].URL.String(), "list followed the next link %q to the wrong address", testCase.next)
		})
	}
}

func TestList_EndsOnlyWithoutANextLink(t *testing.T) {
	// A Pipelines list's pagelen counts the items on the page, so an empty page does not end it.
	transport := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusOK, nil, `{"values": [], "pagelen": 0, "next": "https://api.bitbucket.org/2.0/things?page=2"}`),
		stubResponse(http.StatusOK, nil, `{"values": [{"name": "late"}], "pagelen": 1}`),
	}}
	client := newTestClient(t, transport)

	items, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, nil)
	require.NoError(t, err)

	assert.Equal(t, []item{{Name: "late"}}, items)
}

func TestList_RefusesANextLinkItCannotFollow(t *testing.T) {
	cases := []struct {
		name     string
		next     string
		expected string
	}{
		{
			name:     "outside the 2.0 API without a page number",
			next:     "/rest/1.0/accounts/x/executions?pagelen=10",
			expected: `bitbucket: the list /2.0/things answered a next link outside its API without a page number, "/rest/1.0/accounts/x/executions?pagelen=10"`,
		},
		{
			name:     "outside the 2.0 API with page 0",
			next:     "/rest/1.0/x?page=0",
			expected: `bitbucket: the list /2.0/things answered a next link outside its API without a page number, "/rest/1.0/x?page=0"`,
		},
		{
			name:     "to the page it just gave",
			next:     "https://api.bitbucket.org/2.0/things?pagelen=100",
			expected: `bitbucket: the list /2.0/things answered a next link to a page it already gave, "https://api.bitbucket.org/2.0/things?pagelen=100"`,
		},
		{
			name:     "not an address",
			next:     "%zz",
			expected: `bitbucket: the list /2.0/things answered a next link that is not an address, "%zz": parse "%zz": invalid URL escape "%zz"`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newTestClient(t, &recordingTransport{responses: []*http.Response{
				stubResponse(http.StatusOK, nil, fmt.Sprintf(`{"values": [], "next": %q}`, testCase.next)),
			}})

			_, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, nil)

			assert.EqualError(t, err, testCase.expected, "list refused the next link %q with the wrong error", testCase.next)
		})
	}
}

func TestList_RefusesANextLinkBackToAnEarlierPage(t *testing.T) {
	transport := &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusOK, nil, `{"values": [{"name": "a"}], "next": "https://api.bitbucket.org/2.0/things?pagelen=100&page=2"}`),
		stubResponse(http.StatusOK, nil, `{"values": [{"name": "b"}], "next": "https://api.bitbucket.org/2.0/things?pagelen=100"}`),
	}}
	client := newTestClient(t, transport)

	_, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, nil)

	assert.EqualError(t, err, `bitbucket: the list /2.0/things answered a next link to a page it already gave, "https://api.bitbucket.org/2.0/things?pagelen=100"`)
	assert.Len(t, transport.requests, 2, "list should not ask for page 1 again")
}

func TestList_Options(t *testing.T) {
	cases := []struct {
		name             string
		options          []ListOption
		expectedPagelen  string
		expectedRequests int
		expectedItems    int
	}{
		{
			name:             "every page, of the list's largest size, by default",
			expectedPagelen:  "3",
			expectedRequests: 3,
			expectedItems:    7,
		},
		{
			name:             "a limit asks for pages no larger than itself",
			options:          []ListOption{WithLimit(2)},
			expectedPagelen:  "2",
			expectedRequests: 1,
			expectedItems:    2,
		},
		{
			name:             "a limit stops in the middle of a page",
			options:          []ListOption{WithPageSize(3), WithLimit(4)},
			expectedPagelen:  "3",
			expectedRequests: 2,
			expectedItems:    4,
		},
		{
			name:             "a full walk undoes a limit before it",
			options:          []ListOption{WithLimit(2), WithFullWalk()},
			expectedPagelen:  "3",
			expectedRequests: 3,
			expectedItems:    7,
		},
		{
			name:             "a full walk undoes a limit out of range before it",
			options:          []ListOption{WithLimit(0), WithFullWalk()},
			expectedPagelen:  "3",
			expectedRequests: 3,
			expectedItems:    7,
		},
		{
			name:             "a limit of 1",
			options:          []ListOption{WithLimit(1)},
			expectedPagelen:  "1",
			expectedRequests: 1,
			expectedItems:    1,
		},
		{
			name:             "a limit above the list's length",
			options:          []ListOption{WithLimit(10)},
			expectedPagelen:  "3",
			expectedRequests: 3,
			expectedItems:    7,
		},
		{
			name:             "a smaller page size",
			options:          []ListOption{WithPageSize(1)},
			expectedPagelen:  "1",
			expectedRequests: 7,
			expectedItems:    7,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &pagingTransport{total: 7}
			client := newTestClient(t, transport)

			items, err := list[item](t.Context(), client, []string{"2.0", "things"}, 3, testCase.options)
			require.NoError(t, err)

			assert.Len(t, items, testCase.expectedItems, "list returned the wrong number of items (%s)", testCase.name)
			assert.Equal(t, "item 0", items[0].Name, "list should return the first items (%s)", testCase.name)
			assert.Equal(t, fmt.Sprintf("item %d", len(items)-1), items[len(items)-1].Name, "list should return the items in order (%s)", testCase.name)
			assert.Len(t, transport.requests, testCase.expectedRequests, "list sent the wrong number of requests (%s)", testCase.name)
			assert.Equal(t, testCase.expectedPagelen, transport.requests[0].URL.Query().Get("pagelen"), "list asked for the wrong page size (%s)", testCase.name)
		})
	}
}

func TestList_RefusesAPathPartThatWouldAskForAnotherPath(t *testing.T) {
	transport := &recordingTransport{}
	client := newTestClient(t, transport)

	_, err := list[item](t.Context(), client, []string{"2.0", "workspaces", "", "projects"}, 100, nil)

	assert.EqualError(t, err, `bitbucket: invalid option: the path /2.0/workspaces//projects has a part that is empty, "." or ".."`)
	assert.ErrorIs(t, err, ErrInvalidOption)
	assert.Empty(t, transport.requests)
}

func TestList_RefusesAnOptionOutOfRange(t *testing.T) {
	cases := []struct {
		name     string
		options  []ListOption
		expected string
	}{
		{
			name:     "a page size above the list's largest",
			options:  []ListOption{WithPageSize(101)},
			expected: "bitbucket: invalid option: a page size of 101 is outside 1 to 100",
		},
		{
			name:     "a page size of 0",
			options:  []ListOption{WithPageSize(0)},
			expected: "bitbucket: invalid option: a page size of 0 is outside 1 to 100",
		},
		{
			name:     "a limit of 0",
			options:  []ListOption{WithLimit(0)},
			expected: "bitbucket: invalid option: a list's limit 0 is below 1",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			transport := &recordingTransport{}
			client := newTestClient(t, transport)

			_, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, testCase.options)

			assert.EqualError(t, err, testCase.expected, "list refused %s with the wrong error", testCase.name)
			assert.ErrorIs(t, err, ErrInvalidOption, "list should refuse %s as an invalid option", testCase.name)
			assert.Empty(t, transport.requests, "list should send no request for %s", testCase.name)
		})
	}
}

func TestList_ReturnsTheErrorOfAPage(t *testing.T) {
	client := newTestClient(t, &recordingTransport{responses: []*http.Response{stubResponse(http.StatusNotFound, nil, "")}})

	_, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, nil)

	assert.ErrorIs(t, err, ErrNotFound)
}

// pagingTransport answers a list page by page, as the repository endpoints do.
type pagingTransport struct {
	// total is how many items the list holds.
	total int
	// requests are the requests it got, in the order they came.
	requests []*http.Request
}

func (p *pagingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.requests = append(p.requests, req)

	var pageSize, pageNumber int
	_, _ = fmt.Sscan(req.URL.Query().Get("pagelen"), &pageSize)
	_, _ = fmt.Sscan(req.URL.Query().Get("page"), &pageNumber)
	pageNumber = max(pageNumber, 1)

	start := min((pageNumber-1)*pageSize, p.total)
	end := min(start+pageSize, p.total)
	values := make([]item, 0, end-start)
	for i := start; i < end; i++ {
		values = append(values, item{Name: fmt.Sprintf("item %d", i)})
	}

	answer := map[string]any{"values": values}
	if end < p.total {
		answer["next"] = fmt.Sprintf("https://api.bitbucket.org/2.0/things?pagelen=%d&page=%d", pageSize, pageNumber+1)
	}

	body, _ := json.Marshal(answer)

	return stubResponse(http.StatusOK, nil, string(body)), nil
}

func TestList_NamesThePageAnErrorCameFrom(t *testing.T) {
	client := newTestClient(t, &recordingTransport{responses: []*http.Response{
		stubResponse(http.StatusOK, nil, `{"values": [{"name": "a"}], "next": "https://api.bitbucket.org/2.0/things?page=2"}`),
		stubResponse(http.StatusNotFound, nil, ""),
	}})

	_, err := list[item](t.Context(), client, []string{"2.0", "things"}, 100, nil)

	assert.EqualError(t, err, "bitbucket: GET /2.0/things: 404 Not Found (page 2 of the list)")
	assert.ErrorIs(t, err, ErrNotFound, "a 404 on a later page is still a 404")
}
