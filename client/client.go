package client

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// New's defaults.
const (
	defaultEndpoint      = "https://api.bitbucket.org"
	defaultOAuthTokenURL = "https://bitbucket.org/site/oauth2/access_token" //nolint:gosec // an address, not a credential
	defaultMaxRetries    = 3
	defaultRetryWait     = 200 * time.Millisecond
	// headerTimeout is how long one attempt waits for the answer's headers: a chosen number, not a
	// measured one. A whole request has no limit, since a step log can be large; the caller's context
	// sets the deadline.
	headerTimeout = 60 * time.Second
)

// Client calls Bitbucket Cloud's REST API, with its calls grouped per area as Atlassian's API
// reference groups them: c.Projects.Get, c.Workspaces.ListProjects. A Client is safe for
// concurrent use.
type Client struct {
	// Projects holds the calls of the Projects group.
	Projects *ProjectsArea
	// Workspaces holds the calls of the Workspaces group.
	Workspaces *WorkspacesArea

	// httpClient sends every request through the retries, the login and the log.
	httpClient *http.Client
	// endpoint is the API's address, without a trailing slash.
	endpoint *url.URL
}

// Option changes how New makes a Client.
type Option func(*config)

// config is what the options set.
type config struct {
	// endpoint is the API's address, set by WithEndpoint.
	endpoint string
	// oauthTokenURL is where an OAuth consumer asks for its access token, set by WithOAuthTokenURL.
	oauthTokenURL string
	// httpClient is the client whose transport the Client wraps, set by WithHTTPClient.
	httpClient *http.Client
	// maxRetries is how often an answer that allows it is tried again, set by WithMaxRetries.
	maxRetries int
	// retryWait is the first wait before trying again, set by WithRetryWait.
	retryWait time.Duration
	// logger gets one line per attempt, set by WithLogger; nil logs nothing.
	logger *slog.Logger
}

// WithEndpoint sets the API's address, without "/2.0": https://api.bitbucket.org by default. Tests
// point it at the fake: WithEndpoint(fake.URL).
func WithEndpoint(endpoint string) Option {
	return func(c *config) { c.endpoint = endpoint }
}

// WithOAuthTokenURL sets where an OAuth consumer asks for its access token,
// https://bitbucket.org/site/oauth2/access_token by default; tests point it at the fake.
func WithOAuthTokenURL(tokenURL string) Option {
	return func(c *config) { c.oauthTokenURL = tokenURL }
}

// WithHTTPClient makes the Client send through a copy of httpClient, which stays as it is, and
// every attempt through its transport, if it has one. That transport keeps its own timeouts
// instead of the 60 seconds an attempt otherwise gets to receive the answer's headers.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *config) { c.httpClient = httpClient }
}

// WithMaxRetries sets how often a call is tried again after a 429, 500, 502, 503 or 504, 3 by
// default and 0 for never. A POST is tried again after a 429 only, as a 5xx can come after
// Bitbucket did the work.
func WithMaxRetries(maxRetries int) Option {
	return func(c *config) { c.maxRetries = maxRetries }
}

// WithRetryWait sets the first wait before trying again, 200 milliseconds by default, unless the
// answer's Retry-After says otherwise. It doubles per attempt up to 5 seconds, give or take 20
// percent, so the defaults wait about 200, 400 and 800 milliseconds; 0 tries again at once.
func WithRetryWait(wait time.Duration) Option {
	return func(c *config) { c.retryWait = wait }
}

// WithLogger turns logging on, off by default: one line per attempt, the token requests included,
// at Warn for no answer, a 429 or a 5xx other than 501 and at Debug for any other. A line never
// holds the query, a header or a body, and its duration ends at the answer's headers:
//
//	level=DEBUG msg="bitbucket request" method=GET path=/2.0/workspaces/acme attempt=1 duration=84ms status=200
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) { c.logger = logger }
}

// New makes a Client that logs in with credential, or sends none when it is nil. A credential
// with an empty part, or an option New cannot use, fails with ErrInvalidConfig, such as an http
// endpoint on another machine, which would send the credential unencrypted.
func New(credential Credential, opts ...Option) (*Client, error) {
	cfg := config{
		endpoint:      defaultEndpoint,
		oauthTokenURL: defaultOAuthTokenURL,
		httpClient:    &http.Client{},
		maxRetries:    defaultMaxRetries,
		retryWait:     defaultRetryWait,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	endpoint, err := parseAddress("endpoint", strings.TrimSuffix(cfg.endpoint, "/"))
	if err != nil {
		return nil, err
	}
	if _, err := parseAddress("OAuth token address", cfg.oauthTokenURL); err != nil {
		return nil, err
	}
	if cfg.httpClient == nil {
		return nil, fmt.Errorf("%w: the HTTP client cannot be nil", ErrInvalidConfig)
	}
	if cfg.maxRetries < 0 || cfg.retryWait < 0 {
		return nil, fmt.Errorf("%w: retries %d and retry wait %s cannot be negative", ErrInvalidConfig, cfg.maxRetries, cfg.retryWait)
	}
	if credential != nil {
		if err := credential.check(); err != nil {
			return nil, err
		}
	}

	httpClient := *cfg.httpClient
	httpClient.Transport = transports(credential, cfg, endpoint)

	client := &Client{
		httpClient: &httpClient,
		endpoint:   endpoint,
	}
	client.Projects = &ProjectsArea{client: client}
	client.Workspaces = &WorkspacesArea{client: client}

	return client, nil
}

// transports builds the chain every request goes through: the retries, the login, the log, then
// the transport that sends it. The log sits below the login, so the token request and a request
// sent again after a 401 each get a line of their own.
func transports(credential Credential, cfg config, endpoint *url.URL) http.RoundTripper {
	base := cfg.httpClient.Transport
	if base == nil {
		base = defaultTransport()
	}

	logged := base
	if cfg.logger != nil {
		logged = &loggingTransport{
			next:   base,
			logger: cfg.logger,
		}
	}

	login := logged
	if credential != nil {
		// A second client credentials grant only issues another token, so the token request is
		// tried again after a 5xx too, though it is a POST.
		tokenRetries := newRetryTransport(logged, cfg)
		tokenRetries.repeatable = true

		login = credential.newLogin(loginSettings{
			next:           logged,
			endpoint:       endpoint,
			tokenURL:       cfg.oauthTokenURL,
			tokenTransport: tokenRetries,
		})
	}

	return newRetryTransport(login, cfg)
}

// newRetryTransport returns the retries around next, with the retries and the wait cfg sets.
func newRetryTransport(next http.RoundTripper, cfg config) *retryTransport {
	return &retryTransport{
		next:       next,
		maxRetries: cfg.maxRetries,
		firstWait:  cfg.retryWait,
		randFloat:  rand.Float64,
	}
}

// defaultTransport returns a copy of http.DefaultTransport whose attempts wait at most
// headerTimeout for the answer's headers, or http.DefaultTransport itself when a program replaced
// it with a transport of another kind.
func defaultTransport() http.RoundTripper {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}

	transport = transport.Clone()
	transport.ResponseHeaderTimeout = headerTimeout

	return transport
}

// parseAddress returns address parsed when it is https, or http to this machine, without a user
// or a query, and fails with ErrInvalidConfig otherwise: http elsewhere would carry the credential
// unencrypted, and a user and password would show in every error that names the address.
func parseAddress(name, address string) (*url.URL, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("%w: the %s %q is not an address: %w", ErrInvalidConfig, name, address, err)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: the %s %q holds a user or password; give the credential to New instead", ErrInvalidConfig, name, parsed.Redacted())
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.RawQuery != "" {
		return nil, fmt.Errorf("%w: the %s %q is not an http or https address without a query", ErrInvalidConfig, name, address)
	}
	if parsed.Scheme == "http" && !isLoopback(parsed.Hostname()) {
		return nil, fmt.Errorf("%w: the %s %q would send the credential unencrypted; http is for this machine only", ErrInvalidConfig, name, address)
	}

	return parsed, nil
}

// isLoopback reports whether host is this machine: localhost or a loopback address.
func isLoopback(host string) bool {
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}
