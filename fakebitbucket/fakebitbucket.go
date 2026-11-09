package fakebitbucket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

// The credentials the fake takes, an API token and an OAuth consumer, such as
// client.APIToken{Email: Email, Token: APIToken}. It refuses any other with 401.
//
//nolint:gosec // the fake's own made-up credentials
const (
	Email       = "someone@example.com"
	APIToken    = "fake-api-token"
	OAuthKey    = "fake-oauth-key"
	OAuthSecret = "fake-oauth-secret"
)

// TokenPath is the token address's path: point client.WithOAuthTokenURL at URL + TokenPath.
const TokenPath = "/site/oauth2/access_token" //nolint:gosec // a path, not a credential

// The addresses Bitbucket's links start with: the API's and the web pages'.
const (
	apiAddress = "https://api.bitbucket.org/2.0"
	webAddress = "https://bitbucket.org"
)

// The kinds of credential a Request can carry, told apart by its Authorization header: none, a
// Bearer token, or any other, such as an API token's Basic authentication.
const (
	CredentialNone   = ""
	CredentialBasic  = "basic"
	CredentialBearer = "bearer"
)

// Request is a request the fake received, without its headers and body.
type Request struct {
	// Method is the request's HTTP method.
	Method string
	// Path is the escaped path, as it was sent.
	Path string
	// Query is the request's query.
	Query url.Values
	// Credential is one of the Credential constants.
	Credential string
	// Route is the pattern of the route that answered, such as "GET /2.0/workspaces/{workspace}",
	// or "/" for a path the fake has no route for.
	Route string
}

// Server is the fake Bitbucket, on an httptest.Server whose URL goes to client.WithEndpoint.
type Server struct {
	// Server is the HTTP server the fake runs on.
	*httptest.Server

	// mu guards every field below.
	mu sync.Mutex
	// workspaces holds the seeded workspaces by their slug in lowercase, as Bitbucket finds a
	// workspace by its slug in any case.
	workspaces map[string]models.Workspace
	// projects holds the seeded projects by their workspace's UUID, then by their key, which is
	// case-sensitive.
	projects map[models.UUID]map[string]*storedProject
	// lastSeeded is the number SeedProject gave the project it stored last.
	lastSeeded int
	// tokens holds the access tokens the token address issued that have not expired.
	tokens map[string]bool
	// issued counts the access tokens issued, which makes each one unique.
	issued int
	// failures are the statuses FailNext asked for, for the next requests.
	failures []int
	// requests are the requests the fake received, the first first.
	requests []Request
}

// New starts an empty fake Bitbucket that t stops when the test ends; SeedWorkspace and
// SeedProject fill it.
func New(t testing.TB) *Server {
	t.Helper()

	server := &Server{
		workspaces: map[string]models.Workspace{},
		projects:   map[string]map[string]*storedProject{},
		tokens:     map[string]bool{},
	}

	routes := http.NewServeMux()
	for pattern, handler := range server.routes() {
		routes.HandleFunc(pattern, handler)
	}
	routes.HandleFunc("/", handleNoRoute)

	server.Server = httptest.NewServer(server.intercept(routes))
	t.Cleanup(server.Close)

	return server
}

// routes returns every route the fake serves, by its pattern; TestAnswers sends each one a
// request, so no route escapes the comparison of the fake's answers with Bitbucket's.
func (s *Server) routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		fmt.Sprintf("POST %s", TokenPath):                        s.handleToken,
		"GET /2.0/workspaces/{workspace}":                        s.workspaceRoute(handleGetWorkspace),
		"GET /2.0/workspaces/{workspace}/projects":               s.workspaceRoute(s.handleListProjects),
		"GET /2.0/workspaces/{workspace}/projects/{project_key}": s.workspaceRoute(s.handleGetProject),
	}
}

// FailNext answers the next requests with status, as many as times, before it checks their
// credential, for testing retries: FailNext(503, 2) fails the next two requests with a 503.
func (s *Server) FailNext(status, times int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failures = append(s.failures, slices.Repeat([]int{status}, times)...)
}

// Requests returns every request the fake received, in the order they came.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.requests)
}

// intercept records every request, answers the failures FailNext asked for and refuses a
// credential the fake does not take, before the routes see the request.
func (s *Server) intercept(routes *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := credentialOf(r)
		_, route := routes.Handler(r)
		status, failing := s.record(r, credential, route)
		if failing {
			writeError(w, status, "induced failure")
			return
		}

		// The token address checks the consumer itself.
		if r.URL.Path != TokenPath && credential != CredentialNone && !s.accepts(r) {
			writeError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		routes.ServeHTTP(w, r)
	})
}

// record keeps a request for Requests and returns the status FailNext asked for it, if any.
func (s *Server) record(r *http.Request, credential, route string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, Request{
		Method:     r.Method,
		Path:       r.URL.EscapedPath(),
		Query:      r.URL.Query(),
		Credential: credential,
		Route:      route,
	})
	if len(s.failures) == 0 {
		return 0, false
	}

	status := s.failures[0]
	s.failures = s.failures[1:]

	return status, true
}

// accepts reports whether a request's credential is the fake's API token or a token it issued.
func (s *Server) accepts(r *http.Request) bool {
	if email, token, ok := r.BasicAuth(); ok {
		return email == Email && token == APIToken
	}

	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tokens[token]
}

// credentialOf returns the kind of credential a request carries, one of the Credential constants.
func credentialOf(r *http.Request) string {
	authorization := r.Header.Get("Authorization")
	switch {
	case authorization == "":
		return CredentialNone
	case strings.HasPrefix(authorization, "Bearer "):
		return CredentialBearer
	default:
		return CredentialBasic
	}
}

// visible reports whether a request may see an object: without a credential, Bitbucket shows
// only what is public.
func visible(r *http.Request, isPrivate bool) bool {
	return !isPrivate || credentialOf(r) != CredentialNone
}

// handleNoRoute answers a path the fake has no route for with 501, so a call to a wrong address
// fails instead of looking like a 404.
func handleNoRoute(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, fmt.Sprintf("fakebitbucket has no route for %s %s", r.Method, r.URL.EscapedPath()))
}
