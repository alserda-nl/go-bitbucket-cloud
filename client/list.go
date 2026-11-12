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

// listSettings is what the ListOptions set.
type listSettings struct {
	// pageSize is the page size to ask for.
	pageSize int
	// limit is how many items the list stops after, when limited.
	limit int
	// limited tells a WithLimit(0) from no limit; WithFullWalk clears it.
	limited bool
}

// WithPageSize asks for pages of size items instead of the list's largest page, the default. A
// size below 1 or above the largest page fails with ErrInvalidOption.
func WithPageSize(size int) ListOption {
	return func(s *listSettings) { s.pageSize = size }
}

// WithLimit stops the list after limit items and asks for pages no larger than limit. A limit
// below 1 fails with ErrInvalidOption.
func WithLimit(limit int) ListOption {
	return func(s *listSettings) { s.limit, s.limited = limit, true }
}

// WithFullWalk fetches every page, as a list call does by default; it undoes a WithLimit given before it.
func WithFullWalk() ListOption {
	return func(s *listSettings) { s.limit, s.limited = 0, false }
}

// page is one page of a list as Bitbucket answers it: {"values": [...], "next": "..."}. Only a
// missing next ends a list: pagelen is the page size in some lists and the number of items on the
// page in others, so it decides nothing.
type page[T any] struct {
	// Values are the page's items.
	Values []T `json:"values"`
	// Next is the address of the next page, empty on the last.
	Next string `json:"next"`
}

// list returns the items of the list at the API path made of segments, following next from page
// to page, in pages of largestPage items unless an option asks otherwise. An option out of range
// fails with ErrInvalidOption before any request, and an error on any page fails the whole list.
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
	fetched := map[string]bool{}
	for address, pageNumber := first, 1; ; pageNumber++ {
		fetched[address.String()] = true

		var current page[T]
		if err := c.send(ctx, http.MethodGet, address, nil, &current); err != nil {
			return nil, withPageNumber(err, pageNumber)
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

		// A next link to a page the list already gave would repeat the same requests until ctx ends.
		if fetched[next.String()] {
			return nil, fmt.Errorf("bitbucket: the list %s answered a next link to a page it already gave, %q", first.EscapedPath(), current.Next)
		}

		address = next
	}
}

// withPageNumber names the page in an error from a list's page after the first, as in "(page 3
// of the list)", so the failure does not read like the list's own.
func withPageNumber(err error, pageNumber int) error {
	if pageNumber == 1 {
		return err
	}

	return fmt.Errorf("%w (page %d of the list)", err, pageNumber)
}

// nextPage returns the address of a list's next page: next itself when it points into the 2.0 API
// on the client's own endpoint, and otherwise the list's first address with the page number next
// names, as a schedule's executions answer a relative link outside the 2.0 API while ?page=2 on
// the 2.0 path works. A link without a page number fails.
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
