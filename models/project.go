package models

import "time"

// Project is a Bitbucket project: a group of repositories in a workspace. A repository embeds a
// shorter form, with only the key, links, name, type and UUID.
type Project struct {
	// CreatedOn is when the project was created.
	CreatedOn time.Time `json:"created_on"`
	// Description is the project's description, empty when it has none.
	Description string `json:"description"`
	// HasPubliclyVisibleRepos is true when anyone may see one of the project's repositories.
	HasPubliclyVisibleRepos bool `json:"has_publicly_visible_repos"`
	// IsPrivate is true when only the workspace's members may see the project. A private project
	// holds no public repository.
	IsPrivate bool `json:"is_private"`
	// Key names the project in its workspace and in addresses. It starts with a letter, holds only
	// ASCII letters, digits and underscores, and is case-sensitive: ABC and abc are two projects.
	Key string `json:"key"`
	// Links are the project's picture, its page on bitbucket.org, its repositories and its address
	// in the API.
	Links ProjectLinks `json:"links"`
	// Name is the project's name, unique in its workspace.
	Name string `json:"name"`
	// Owner is the team account of the project's workspace.
	Owner Account `json:"owner"`
	// Type is "project".
	Type string `json:"type"`
	// UpdatedOn is when the project last changed.
	UpdatedOn time.Time `json:"updated_on"`
	// UUID is the project's UUID, which never changes, not even with its key.
	UUID UUID `json:"uuid"`
	// Workspace is the project's workspace, in the shorter form another object embeds.
	Workspace Workspace `json:"workspace"`
}

// ProjectLinks are a project's links.
type ProjectLinks struct {
	// Avatar is the address of the project's picture; its ts query changes whenever the project does.
	Avatar Link `json:"avatar"`
	// HTML is the project's page on bitbucket.org.
	HTML Link `json:"html"`
	// Repositories is the API address that lists the project's repositories.
	Repositories Link `json:"repositories"`
	// Self is the project's address in the API.
	Self Link `json:"self"`
}
