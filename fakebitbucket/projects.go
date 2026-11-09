package fakebitbucket

import (
	"cmp"
	_ "embed"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

const (
	// projectsDefaultPage is the projects list's page size when none is asked.
	projectsDefaultPage = 50
	// projectsLargestPage is the largest it takes: Bitbucket refuses 101 with 400.
	projectsLargestPage = 100
)

//go:embed samples/project.json
var projectSample []byte

// projectNotFoundSample is Bitbucket's answer for a project that is not there, or not visible.
//
//go:embed samples/errors/project-not-found.json
var projectNotFoundSample []byte

// ProjectOption changes a project NewProject makes.
type ProjectOption func(*models.Project)

// ProjectOptionSet holds the options of NewProject: fakebitbucket.Project.WithKey("ABC").
type ProjectOptionSet struct{}

// Project is NewProject's options.
//
//nolint:gochecknoglobals // the options' namespace: fakebitbucket.Project.WithKey
var Project ProjectOptionSet

// NewProject returns a private project as Bitbucket answers one, with a key, name and UUID no
// other builder call returns. Its owner and links follow its workspace and key.
func NewProject(opts ...ProjectOption) models.Project {
	var project models.Project
	decodeSample(projectSample, &project)

	id := nextID()
	project.UUID = uuidFor(id)
	project.Key = fmt.Sprintf("P%d", id)
	project.Name = fmt.Sprintf("Project %d", id)

	for _, opt := range opts {
		opt(&project)
	}

	project.Owner = ownerOf(project.Workspace, project.Owner.Username)
	project.Links = projectLinks(project.Workspace.Slug, project.Key)

	return project
}

// WithKey sets the project's key.
func (ProjectOptionSet) WithKey(key string) ProjectOption {
	return func(p *models.Project) { p.Key = key }
}

// WithPrivate sets whether the project is private.
func (ProjectOptionSet) WithPrivate(isPrivate bool) ProjectOption {
	return func(p *models.Project) { p.IsPrivate = isPrivate }
}

// WithWorkspace puts the project in workspace.
func (ProjectOptionSet) WithWorkspace(workspace models.Workspace) ProjectOption {
	return func(p *models.Project) { p.Workspace = workspace }
}

// storedProject is a project as the fake keeps it, with what it needs to answer as Bitbucket does.
type storedProject struct {
	// project is the project as SeedProject stored it.
	project models.Project
	// seeded is the project's place in the order SeedProject stored projects in.
	seeded int
	// hiddenReads counts the reads the project still answers 404 to.
	hiddenReads int
}

// SeedProject stores a project in its workspace and returns it. It panics when SeedWorkspace did
// not store that workspace, as HideProject does for a project SeedProject did not store.
func (s *Server) SeedProject(project models.Project) models.Project {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, found := s.findWorkspace(project.Workspace.UUID); !found {
		panic(fmt.Sprintf("fakebitbucket: SeedProject: SeedWorkspace stored no workspace %s", project.Workspace.Slug))
	}

	workspaceUUID := project.Workspace.UUID
	if s.projects[workspaceUUID] == nil {
		s.projects[workspaceUUID] = map[string]*storedProject{}
	}

	s.lastSeeded++
	s.projects[workspaceUUID][project.Key] = &storedProject{
		project: project,
		seeded:  s.lastSeeded,
	}

	return project
}

// HideProject makes a project SeedProject stored answer 404 to its next reads, as many as reads,
// as a new project can for 38 seconds and more after its create.
func (s *Server) HideProject(project models.Project, reads int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, found := s.projects[project.Workspace.UUID][project.Key]
	if !found {
		panic(fmt.Sprintf("fakebitbucket: HideProject: SeedProject stored no project %s in %s", project.Key, project.Workspace.Slug))
	}

	stored.hiddenReads = reads
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request, workspace models.Workspace) {
	stored, found := s.projects[workspace.UUID][r.PathValue("project_key")]
	if found && stored.hiddenReads > 0 {
		stored.hiddenReads--
		writeSample(w, projectNotFoundSample)
		return
	}
	if !found || !visible(r, stored.project.IsPrivate) {
		writeSample(w, projectNotFoundSample)
		return
	}

	writeJSON(w, http.StatusOK, projectAnswer(stored.project, workspace))
}

// handleListProjects lists the newest project first, as Bitbucket does; here the newest is the one
// seeded last.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, workspace models.Workspace) {
	newestFirst := func(a, b *storedProject) int {
		return cmp.Compare(b.seeded, a.seeded)
	}

	var answers []any
	for _, stored := range slices.SortedFunc(maps.Values(s.projects[workspace.UUID]), newestFirst) {
		if visible(r, stored.project.IsPrivate) {
			answers = append(answers, projectAnswer(stored.project, workspace))
		}
	}

	writePage(w, r, answers, projectsDefaultPage, projectsLargestPage)
}

// projectAnswer is a project as Bitbucket answers it: its owner and its workspace in the short
// forms Bitbucket embeds, made from the workspace as stored now, so a workspace seeded again shows.
func projectAnswer(project models.Project, workspace models.Workspace) any {
	return struct {
		models.Project
		Owner     teamReference      `json:"owner"`
		Workspace workspaceReference `json:"workspace"`
	}{
		Project:   project,
		Owner:     teamReferenceTo(ownerOf(workspace, project.Owner.Username)),
		Workspace: workspaceReferenceTo(workspace),
	}
}

// projectLinks returns a project's links as Bitbucket writes them.
func projectLinks(workspaceSlug, key string) models.ProjectLinks {
	html := fmt.Sprintf("https://bitbucket.org/%s/workspace/projects/%s", workspaceSlug, key)

	return models.ProjectLinks{
		Avatar:       models.Link{Href: fmt.Sprintf("%s/avatar/32", html)},
		HTML:         models.Link{Href: html},
		Repositories: models.Link{Href: fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s?q=project.key=%q", workspaceSlug, key)},
		Self:         models.Link{Href: fmt.Sprintf("https://api.bitbucket.org/2.0/workspaces/%s/projects/%s", workspaceSlug, key)},
	}
}
