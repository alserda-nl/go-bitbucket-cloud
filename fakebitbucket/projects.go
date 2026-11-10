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

// The page sizes of Bitbucket's projects list.
const (
	// projectsDefaultPage is the projects list's page size when none is asked.
	projectsDefaultPage = 50
	// projectsLargestPage is the largest it takes: Bitbucket refuses 101 with 400.
	projectsLargestPage = 100
)

// projectSample is Bitbucket's answer for a project, which NewProject starts from.
//
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

// Project is how a test reaches NewProject's options: NewProject(fakebitbucket.Project.WithKey("ABC")).
//
//nolint:gochecknoglobals // the options' namespace: fakebitbucket.Project.WithKey
var Project ProjectOptionSet

// NewProject returns a private project as Bitbucket answers one, with a key, name and UUID no
// other builder call returns; its owner and links follow its workspace and key. It stores nothing:
// SeedProject puts it in a fake.
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

// WithKey sets the project's key, and so its links.
func (ProjectOptionSet) WithKey(key string) ProjectOption {
	return func(p *models.Project) { p.Key = key }
}

// WithPrivate sets whether the project is private: WithPrivate(false) makes it public, where
// NewProject's projects are private.
func (ProjectOptionSet) WithPrivate(isPrivate bool) ProjectOption {
	return func(p *models.Project) { p.IsPrivate = isPrivate }
}

// WithWorkspace puts the project in workspace, which also gives it its owner and links.
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

// SeedProject stores a project in its workspace, as if it existed in Bitbucket, and returns it.
// It panics when SeedWorkspace did not store that workspace first.
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

// HideProject makes the next reads of a stored project answer 404, as many as reads, and keeps it
// out of the projects list until then without spending one, as Bitbucket can for a while after its
// create: HideProject(project, 2) hides it from the next two reads. It panics for a project
// SeedProject did not store.
func (s *Server) HideProject(project models.Project, reads int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, found := s.projects[project.Workspace.UUID][project.Key]
	if !found {
		panic(fmt.Sprintf("fakebitbucket: HideProject: SeedProject stored no project %s in %s", project.Key, project.Workspace.Slug))
	}

	stored.hiddenReads = reads
}

// handleGetProject answers the project the path names, its key case-sensitive, or Bitbucket's 404
// for one that is not there, is hidden, or is private and asked for without a credential.
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

// handleListProjects lists a workspace's projects newest first, as Bitbucket does, the one seeded
// last counting as the newest. It leaves out a hidden project, and a private one for a request
// without a credential.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request, workspace models.Workspace) {
	newestFirst := func(a, b *storedProject) int {
		return cmp.Compare(b.seeded, a.seeded)
	}

	var answers []any
	for _, stored := range slices.SortedFunc(maps.Values(s.projects[workspace.UUID]), newestFirst) {
		// A hidden project keeps its reads: only a read of the project itself spends one.
		if stored.hiddenReads > 0 || !visible(r, stored.project.IsPrivate) {
			continue
		}

		answers = append(answers, projectAnswer(stored.project, workspace))
	}

	writePage(w, r, answers, projectsDefaultPage, projectsLargestPage)
}

// projectAnswer is a project as Bitbucket answers it, its owner and its workspace in the short
// forms Bitbucket embeds, both made from workspace as it is stored, so a workspace seeded again
// shows in its projects.
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

// projectLinks returns a project's links as Bitbucket writes them: for ("acme", "ABC") its Self
// is https://api.bitbucket.org/2.0/workspaces/acme/projects/ABC.
func projectLinks(workspaceSlug, key string) models.ProjectLinks {
	html := fmt.Sprintf("%s/%s/workspace/projects/%s", webAddress, workspaceSlug, key)

	return models.ProjectLinks{
		Avatar:       models.Link{Href: fmt.Sprintf("%s/avatar/32", html)},
		HTML:         models.Link{Href: html},
		Repositories: models.Link{Href: fmt.Sprintf("%s/repositories/%s?q=project.key=%q", apiAddress, workspaceSlug, key)},
		Self:         models.Link{Href: fmt.Sprintf("%s/workspaces/%s/projects/%s", apiAddress, workspaceSlug, key)},
	}
}
