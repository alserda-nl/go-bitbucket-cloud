package models

import "time"

// Project is a Bitbucket project. A repository embeds only its key, links, name, type and UUID
// (2026-10-14).
type Project struct {
	CreatedOn               time.Time `json:"created_on"`
	Description             string    `json:"description"`
	HasPubliclyVisibleRepos bool      `json:"has_publicly_visible_repos"`
	IsPrivate               bool      `json:"is_private"`
	// Key is case-sensitive: a read of the key RPCASE answered 404 for the project rpcase (2026-10-14).
	Key       string       `json:"key"`
	Links     ProjectLinks `json:"links"`
	Name      string       `json:"name"`
	Owner     Account      `json:"owner"`
	Type      string       `json:"type"`
	UpdatedOn time.Time    `json:"updated_on"`
	UUID      UUID         `json:"uuid"`
	Workspace Workspace    `json:"workspace"`
}

// ProjectLinks are a project's links.
type ProjectLinks struct {
	Avatar       Link `json:"avatar"`
	HTML         Link `json:"html"`
	Repositories Link `json:"repositories"`
	Self         Link `json:"self"`
}
