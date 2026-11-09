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
// other builder call returns. A workspace's name need not be its slug.
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
// A UUID without braces finds nothing.
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

// writeWorkspaceNotFound answers as Bitbucket answers a workspace that does not exist.
func writeWorkspaceNotFound(w http.ResponseWriter, identifier string) {
	writeError(w, http.StatusNotFound, fmt.Sprintf("No workspace with identifier '%s'.", identifier))
}

// workspaceLinks returns a workspace's links as Bitbucket writes them.
func workspaceLinks(slug string) models.WorkspaceLinks {
	api := fmt.Sprintf("%s/workspaces/%s", apiAddress, slug)

	return models.WorkspaceLinks{
		Avatar:       models.Link{Href: fmt.Sprintf("%s/workspaces/%s/avatar/", webAddress, slug)},
		Hooks:        models.Link{Href: fmt.Sprintf("%s/hooks", api)},
		HTML:         models.Link{Href: fmt.Sprintf("%s/%s/", webAddress, slug)},
		HTMLOverview: models.Link{Href: fmt.Sprintf("%s/%s/workspace/overview/", webAddress, slug)},
		Members:      models.Link{Href: fmt.Sprintf("%s/members", api)},
		Owners:       models.Link{Href: fmt.Sprintf("%s/members?q=permission%%3D%%22owner%%22", api)},
		Projects:     models.Link{Href: fmt.Sprintf("%s/projects", api)},
		Repositories: models.Link{Href: fmt.Sprintf("%s/repositories/%s", apiAddress, slug)},
		Self:         models.Link{Href: api},
		Snippets:     models.Link{Href: fmt.Sprintf("%s/snippets/%s", apiAddress, slug)},
	}
}

// workspaceReference is the short form of a workspace that Bitbucket embeds in another object.
type workspaceReference struct {
	// Links are the workspace's picture, page and address in the API.
	Links referenceLinks `json:"links"`
	// Name is the workspace's name.
	Name string `json:"name"`
	// Slug is the workspace's slug.
	Slug string `json:"slug"`
	// Type is "workspace".
	Type string `json:"type"`
	// UUID is the workspace's UUID.
	UUID models.UUID `json:"uuid"`
}

// referenceLinks are the links of an embedded workspace.
type referenceLinks struct {
	// Avatar is the address of the workspace's picture.
	Avatar models.Link `json:"avatar"`
	// HTML is the workspace's page on bitbucket.org.
	HTML models.Link `json:"html"`
	// Self is the workspace's address in the API.
	Self models.Link `json:"self"`
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
// and no account ID or nickname.
type teamReference struct {
	// DisplayName is the team's name, its workspace's name.
	DisplayName string `json:"display_name"`
	// Links are the team's picture, page and address in the API.
	Links models.AccountLinks `json:"links"`
	// Type is "team".
	Type string `json:"type"`
	// Username is the team's username.
	Username string `json:"username"`
	// UUID is the team's UUID, its workspace's UUID.
	UUID models.UUID `json:"uuid"`
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
// workspace itself, under the team's username, which need not be the workspace's slug.
func ownerOf(workspace models.Workspace, username string) models.Account {
	escapedUUID := url.PathEscape(workspace.UUID)

	return models.Account{
		DisplayName: workspace.Name,
		Links: models.AccountLinks{
			Avatar: models.Link{Href: fmt.Sprintf("%s/account/%s/avatar/", webAddress, username)},
			HTML:   models.Link{Href: fmt.Sprintf("%s/%s/", webAddress, escapedUUID)},
			Self:   models.Link{Href: fmt.Sprintf("%s/workspaces/%s", apiAddress, escapedUUID)},
		},
		Type:     "team",
		Username: username,
		UUID:     workspace.UUID,
	}
}
