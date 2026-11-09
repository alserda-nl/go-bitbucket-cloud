// Package client is a Go client for Bitbucket Cloud's REST API. Its runtime code depends on the Go
// standard library only.
//
// New makes a Client from one credential, an API token or an OAuth consumer, and options; the
// Client's calls are grouped per area, as Atlassian's API reference groups them. Every answer of
// 400 or above comes back as an *APIError, and errors.Is(err, ErrNotFound) is true for every 404:
//
//	c, err := client.New(client.APIToken{Email: "someone@example.com", Token: token})
//	project, err := c.Projects.Get(ctx, "acme", "ABC")
package client
