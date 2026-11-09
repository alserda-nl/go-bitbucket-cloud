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

// Client calls Bitbucket Cloud's REST API. Its calls are grouped per area, as Atlassian's API
// reference groups them.
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

// WithEndpoint sets the API's address, https://api.bitbucket.org by default; tests point it at the fake.
func WithEndpoint(endpoint string) Option {
	return func(c *config) { c.endpoint = endpoint }
}

// WithOAuthTokenURL sets where an OAuth consumer asks for its access token,
// https://bitbucket.org/site/oauth2/access_token by default; tests point it at the fake.
func WithOAuthTokenURL(tokenURL string) Option {
	return func(c *config) { c.oauthTokenURL = tokenURL }
}

// WithHTTPClient sets the *http.Client whose transport the login, the retries and the logging wrap.
// It is copied, never changed. Its transport's timeouts then apply instead of the 60 seconds an
// attempt otherwise gets to receive the answer's headers.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *config) { c.httpClient = httpClient }
}

// WithMaxRetries sets how many times an answer that allows it is tried again, 3 by default; 0 never.
func WithMaxRetries(maxRetries int) Option {
	return func(c *config) { c.maxRetries = maxRetries }
}

// WithRetryWait sets the first wait before trying again, 200 milliseconds by default; it doubles
// per attempt up to 5 seconds, and 0 tries again at once.
func WithRetryWait(wait time.Duration) Option {
	return func(c *config) { c.retryWait = wait }
}

// WithLogger turns logging on: one line per attempt, the token request's and a request sent again
// after a 401 included, at Warn for an attempt that got no answer, a 429 or a 5xx and at Debug for
// any other. An attempt's duration ends at the answer's headers. Logging is off by default.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) { c.logger = logger }
}

// New makes a Client that logs in with credential, which may be nil to send none.
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

// parseAddress checks that address is an absolute http or https address without a query, and that
// an http one goes to this machine: the credential goes to it unencrypted, as it does to the fake.
func parseAddress(name, address string) (*url.URL, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("%w: the %s %q is not an address: %w", ErrInvalidConfig, name, address, err)
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
