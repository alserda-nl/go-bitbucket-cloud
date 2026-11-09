package models

import (
	"encoding/json"
	"time"
)

// The forking modes a workspace documents.
const (
	// ForkingModeAllowForks lets every repository be forked anywhere.
	ForkingModeAllowForks = "allow_forks"
	// ForkingModeInternalOnly keeps forks of private repositories inside the workspace and private.
	ForkingModeInternalOnly = "internal_only"
)

// Workspace is a Bitbucket workspace, which holds projects and repositories. Another object embeds
// a shorter form, with only the links (avatar, HTML and self), name, slug, type and UUID.
type Workspace struct {
	// CreatedOn is when the workspace was created.
	CreatedOn time.Time `json:"created_on"`
	// ForkingMode says where the workspace's repositories may be forked to: one of the ForkingMode
	// constants, or a value Bitbucket adds later.
	ForkingMode string `json:"forking_mode"`
	// IsPersonal is true for a workspace that belongs to one user.
	IsPersonal bool `json:"is_personal"`
	// IsPrivacyEnforced is true when the workspace allows no public content, such as a public repository.
	IsPrivacyEnforced bool `json:"is_privacy_enforced"`
	// IsPrivate is true when the workspace's content is for its members only; Bitbucket still answers
	// the workspace's own details to anyone.
	IsPrivate bool `json:"is_private"`
	// Links are the workspace's picture, its pages on bitbucket.org and the API addresses of its
	// members, owners, projects, repositories, snippets and webhooks.
	Links WorkspaceLinks `json:"links"`
	// Name is the workspace's name as Bitbucket shows it, which need not be its slug.
	Name string `json:"name"`
	// SignSystemCommits is left out of Bitbucket's specification and its answers hold null, so its
	// type is unknown and it stays raw JSON.
	SignSystemCommits json.RawMessage `json:"sign_system_commits"`
	// Slug names the workspace in addresses, such as acme in bitbucket.org/acme. It is lowercase,
	// and Bitbucket finds the workspace by it in any case.
	Slug string `json:"slug"`
	// Type is "workspace".
	Type string `json:"type"`
	// UUID is the workspace's UUID, which never changes.
	UUID UUID `json:"uuid"`
}

// WorkspaceLinks are a workspace's links.
type WorkspaceLinks struct {
	// Avatar is the address of the workspace's picture.
	Avatar Link `json:"avatar"`
	// Hooks is the API address of the workspace's webhooks.
	Hooks Link `json:"hooks"`
	// HTML is the workspace's page on bitbucket.org.
	HTML Link `json:"html"`
	// HTMLOverview is the workspace's overview page on bitbucket.org.
	HTMLOverview Link `json:"html_overview"`
	// Members is the API address that lists the workspace's members.
	Members Link `json:"members"`
	// Owners is the API address that lists the members with the owner permission.
	Owners Link `json:"owners"`
	// Projects is the API address that lists the workspace's projects.
	Projects Link `json:"projects"`
	// Repositories is the API address that lists the workspace's repositories.
	Repositories Link `json:"repositories"`
	// Self is the workspace's address in the API.
	Self Link `json:"self"`
	// Snippets is the API address that lists the workspace's snippets.
	Snippets Link `json:"snippets"`
}
