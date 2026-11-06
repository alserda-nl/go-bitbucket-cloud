package fakebitbucket

import (
	_ "embed"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

//go:embed samples/workspace.json
var workspaceSample []byte

// WorkspaceOption changes a workspace NewWorkspace makes.
type WorkspaceOption func(*models.Workspace)

// WorkspaceOptionSet holds the options of NewWorkspace: fakebitbucket.Workspace.WithSlug("acme").
type WorkspaceOptionSet struct{}

// Workspace is NewWorkspace's options.
//
//nolint:gochecknoglobals // the options' namespace: fakebitbucket.Workspace.WithSlug
var Workspace WorkspaceOptionSet

// NewWorkspace returns a workspace as Bitbucket answers one, with a slug, a name and a UUID no
// other builder call returns. A workspace's name need not be its slug (2026-10-12).
func NewWorkspace(opts ...WorkspaceOption) models.Workspace {
	var workspace models.Workspace
	decodeSample(workspaceSample, &workspace)

	id := nextID()
	workspace.UUID = uuidFor(id)
	workspace.Slug = fmt.Sprintf("workspace-%d", id)
	workspace.Name = fmt.Sprintf("Workspace %d", id)

	for _, opt := range opts {
		opt(&workspace)
	}

	workspace.Links = workspaceLinks(workspace.Slug)

	return workspace
}

// WithSlug sets the workspace's slug, and so its links.
func (WorkspaceOptionSet) WithSlug(slug string) WorkspaceOption {
	return func(w *models.Workspace) { w.Slug = slug }
}

// SeedWorkspace stores a workspace, as if it existed in Bitbucket, and returns it.
func (s *Server) SeedWorkspace(workspace models.Workspace) models.Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.workspaces[strings.ToLower(workspace.Slug)] = workspace

	return workspace
}

// workspaceRoute serves a route under /2.0/workspaces/{workspace}: it holds the lock, finds the
// workspace by its slug or its UUID in braces, and answers as Bitbucket does when there is none.
func (s *Server) workspaceRoute(handle func(http.ResponseWriter, *http.Request, models.Workspace)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()

		workspace, found := s.findWorkspace(r.PathValue("workspace"))
		if !found {
			writeWorkspaceNotFound(w, r.PathValue("workspace"))
			return
		}

		handle(w, r, workspace)
	}
}

func handleGetWorkspace(w http.ResponseWriter, _ *http.Request, workspace models.Workspace) {
	writeJSON(w, http.StatusOK, workspace)
}

// findWorkspace finds a workspace by its slug, in any case, or its UUID in braces; s.mu is held.
// A UUID without braces finds nothing (2026-10-19).
func (s *Server) findWorkspace(identifier string) (models.Workspace, bool) {
	if workspace, found := s.workspaces[strings.ToLower(identifier)]; found {
		return workspace, true
	}
	for _, workspace := range s.workspaces {
		if strings.EqualFold(workspace.UUID, identifier) {
			return workspace, true
		}
	}

	return models.Workspace{}, false
}

// writeWorkspaceNotFound answers as Bitbucket answered a workspace that does not exist (2026-10-19).
func writeWorkspaceNotFound(w http.ResponseWriter, identifier string) {
	writeError(w, http.StatusNotFound, fmt.Sprintf("No workspace with identifier '%s'.", identifier))
}

// workspaceLinks returns a workspace's links as Bitbucket writes them (2026-10-19).
func workspaceLinks(slug string) models.WorkspaceLinks {
	api := fmt.Sprintf("https://api.bitbucket.org/2.0/workspaces/%s", slug)

	return models.WorkspaceLinks{
		Avatar:       models.Link{Href: fmt.Sprintf("https://bitbucket.org/workspaces/%s/avatar/", slug)},
		Hooks:        models.Link{Href: fmt.Sprintf("%s/hooks", api)},
		HTML:         models.Link{Href: fmt.Sprintf("https://bitbucket.org/%s/", slug)},
		HTMLOverview: models.Link{Href: fmt.Sprintf("https://bitbucket.org/%s/workspace/overview/", slug)},
		Members:      models.Link{Href: fmt.Sprintf("%s/members", api)},
		Owners:       models.Link{Href: fmt.Sprintf("%s/members?q=permission%%3D%%22owner%%22", api)},
		Projects:     models.Link{Href: fmt.Sprintf("%s/projects", api)},
		Repositories: models.Link{Href: fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s", slug)},
		Self:         models.Link{Href: api},
		Snippets:     models.Link{Href: fmt.Sprintf("https://api.bitbucket.org/2.0/snippets/%s", slug)},
	}
}

// workspaceReference is the short form of a workspace that Bitbucket embeds in another object.
type workspaceReference struct {
	Links referenceLinks `json:"links"`
	Name  string         `json:"name"`
	Slug  string         `json:"slug"`
	Type  string         `json:"type"`
	UUID  models.UUID    `json:"uuid"`
}

// referenceLinks are the links of an embedded workspace.
type referenceLinks struct {
	Avatar models.Link `json:"avatar"`
	HTML   models.Link `json:"html"`
	Self   models.Link `json:"self"`
}

func workspaceReferenceTo(workspace models.Workspace) workspaceReference {
	return workspaceReference{
		Links: referenceLinks{
			Avatar: workspace.Links.Avatar,
			HTML:   workspace.Links.HTML,
			Self:   workspace.Links.Self,
		},
		Name: workspace.Name,
		Slug: workspace.Slug,
		Type: workspace.Type,
		UUID: workspace.UUID,
	}
}

// teamReference is a team account as Bitbucket embeds one, a project's owner: it has a username,
// and no account ID or nickname (2026-10-14).
type teamReference struct {
	DisplayName string              `json:"display_name"`
	Links       models.AccountLinks `json:"links"`
	Type        string              `json:"type"`
	Username    string              `json:"username"`
	UUID        models.UUID         `json:"uuid"`
}

func teamReferenceTo(account models.Account) teamReference {
	return teamReference{
		DisplayName: account.DisplayName,
		Links:       account.Links,
		Type:        account.Type,
		Username:    account.Username,
		UUID:        account.UUID,
	}
}

// ownerOf returns the team account Bitbucket names as the owner of a workspace's projects: the
// workspace itself, under the team's username, which need not be the workspace's slug (2026-10-14).
func ownerOf(workspace models.Workspace, username string) models.Account {
	escapedUUID := url.PathEscape(workspace.UUID)

	return models.Account{
		DisplayName: workspace.Name,
		Links: models.AccountLinks{
			Avatar: models.Link{Href: fmt.Sprintf("https://bitbucket.org/account/%s/avatar/", username)},
			HTML:   models.Link{Href: fmt.Sprintf("https://bitbucket.org/%s/", escapedUUID)},
			Self:   models.Link{Href: fmt.Sprintf("https://api.bitbucket.org/2.0/workspaces/%s", escapedUUID)},
		},
		Type:     "team",
		Username: username,
		UUID:     workspace.UUID,
	}
}
