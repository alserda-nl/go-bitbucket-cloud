// Package client is a Go client for Bitbucket Cloud's REST API. Its runtime code depends on the Go
// standard library only.
//
// New makes a Client from one credential, an API token or an OAuth consumer, and options. Its
// calls are grouped per area, as the groups of Atlassian's API reference: c.Projects.Get,
// c.Workspaces.Get. Every answer of 400 or above comes back as an *APIError.
package client
