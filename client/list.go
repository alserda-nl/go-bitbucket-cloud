package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ListOption changes what a list call asks for.
type ListOption func(*listSettings)

type listSettings struct {
	pageSize int
	limit    int
	// limited tells a WithLimit(0) from no limit; WithFullWalk clears it.
	limited bool
}

// WithPageSize asks for pages of size items; it is at most the list's largest page, which a list
// call asks for by default.
func WithPageSize(size int) ListOption {
	return func(s *listSettings) { s.pageSize = size }
}

// WithLimit stops the list after limit items and asks for pages no larger than limit.
func WithLimit(limit int) ListOption {
	return func(s *listSettings) { s.limit, s.limited = limit, true }
}

// WithFullWalk fetches every page, as a list call does by default; it undoes a WithLimit given before it.
func WithFullWalk() ListOption {
	return func(s *listSettings) { s.limit, s.limited = 0, false }
}

// page is the wrapper every list answers with. Only a missing next ends a list: pagelen is the
// page size in some lists and the number of items on the page in others (2026-10-26), so it
// decides nothing.
type page[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

// list fetches the list at the API path made of segments, page by page, asking for pages of the
// list's largest size unless an option says otherwise.
func list[T any](ctx context.Context, c *Client, segments []string, largestPage int, opts []ListOption) ([]T, error) {
	settings := listSettings{pageSize: largestPage}
	for _, opt := range opts {
		opt(&settings)
	}

	if settings.limited && settings.limit < 1 {
		return nil, fmt.Errorf("%w: a list's limit %d is below 1", ErrInvalidOption, settings.limit)
	}
	if settings.pageSize < 1 || settings.pageSize > largestPage {
		return nil, fmt.Errorf("%w: a page size of %d is outside 1 to %d", ErrInvalidOption, settings.pageSize, largestPage)
	}

	pageSize := settings.pageSize
	if settings.limited {
		pageSize = min(pageSize, settings.limit)
	}
	first, err := c.apiURL(segments, url.Values{"pagelen": {strconv.Itoa(pageSize)}})
	if err != nil {
		return nil, err
	}

	var items []T
	for address, pageNumber := first, 1; ; pageNumber++ {
		var current page[T]
		if err := c.send(ctx, http.MethodGet, address, nil, &current); err != nil {
			return nil, onPage(err, pageNumber)
		}

		items = append(items, current.Values...)
		if settings.limited && len(items) >= settings.limit {
			return items[:settings.limit], nil
		}
		if current.Next == "" {
			return items, nil
		}

		next, err := c.nextPage(first, current.Next)
		if err != nil {
			return nil, err
		}

		address = next
	}
}

// onPage adds the page to an error from a list's page after the first, so a failure there does not
// read like the list's own.
func onPage(err error, pageNumber int) error {
	if pageNumber == 1 {
		return err
	}

	return fmt.Errorf("%w (page %d of the list)", err, pageNumber)
}

// nextPage returns the address of a list's next page. A next link into the 2.0 API on the
// client's own endpoint is followed as given. For any other link it asks the list's own path for
// the page number the link names: a schedule's executions answer a relative link outside the 2.0
// API, while ?page=2 on the 2.0 path works (2026-10-26). A link without a page number ends the
// list with an error.
func (c *Client) nextPage(first *url.URL, next string) (*url.URL, error) {
	link, err := c.endpoint.Parse(next)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: the list %s answered a next link that is not an address, %q: %w", first.EscapedPath(), next, err)
	}

	ownAPI := fmt.Sprintf("%s/%s/", c.endpoint.EscapedPath(), apiVersion)
	if onEndpoint(link, c.endpoint) && strings.HasPrefix(link.EscapedPath(), ownAPI) {
		return link, nil
	}

	pageNumber, err := strconv.Atoi(link.Query().Get("page"))
	if err != nil || pageNumber < 1 {
		return nil, fmt.Errorf("bitbucket: the list %s answered a next link outside its API without a page number, %q", first.EscapedPath(), next)
	}

	query := first.Query()
	query.Set("page", strconv.Itoa(pageNumber))
	page := *first
	page.RawQuery = query.Encode()

	return &page, nil
}
