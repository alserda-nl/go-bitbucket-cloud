package models

import (
	"encoding/json"
	"time"
)

// The forking modes a workspace documents.
const (
	ForkingModeAllowForks   = "allow_forks"
	ForkingModeInternalOnly = "internal_only"
)

// Workspace is a Bitbucket workspace. Another object embeds only its links (avatar, HTML and
// self), name, slug, type and UUID (2026-10-14).
type Workspace struct {
	CreatedOn time.Time `json:"created_on"`
	// ForkingMode is one of the ForkingMode constants, or a value Bitbucket added since.
	ForkingMode       string         `json:"forking_mode"`
	IsPersonal        bool           `json:"is_personal"`
	IsPrivacyEnforced bool           `json:"is_privacy_enforced"`
	IsPrivate         bool           `json:"is_private"`
	Links             WorkspaceLinks `json:"links"`
	Name              string         `json:"name"`
	// SignSystemCommits is missing from Bitbucket's specification, and every answer up to
	// 2026-10-30 held null.
	SignSystemCommits json.RawMessage `json:"sign_system_commits"`
	Slug              string          `json:"slug"`
	Type              string          `json:"type"`
	UUID              UUID            `json:"uuid"`
}

// WorkspaceLinks are a workspace's links.
type WorkspaceLinks struct {
	Avatar       Link `json:"avatar"`
	Hooks        Link `json:"hooks"`
	HTML         Link `json:"html"`
	HTMLOverview Link `json:"html_overview"`
	Members      Link `json:"members"`
	Owners       Link `json:"owners"`
	Projects     Link `json:"projects"`
	Repositories Link `json:"repositories"`
	Self         Link `json:"self"`
	Snippets     Link `json:"snippets"`
}
