package client

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// A Credential logs a Client in to Bitbucket: an APIToken or an OAuthConsumer. A nil Credential
// sends none, and Bitbucket then answers as to anyone: a private workspace's details, a projects
// list of only its public projects, and 404 for a private project (2026-10-30).
type Credential interface {
	// check returns ErrInvalidConfig when a part of the credential is missing.
	check() error
	// newLogin returns the transport that adds the credential to a request.
	newLogin(login loginSettings) http.RoundTripper
}

// APIToken is an Atlassian account's API token, sent with the account's email address as Basic
// authentication.
type APIToken struct {
	Email string
	Token string
}

// OAuthConsumer is a workspace's OAuth consumer: the Client trades its key and secret for an
// access token with the client credentials grant.
type OAuthConsumer struct {
	Key    string
	Secret string
}

// hidden stands in for a token or a secret wherever a credential is printed or logged.
const hidden = "(hidden)"

// String shows the email address and hides the token, so printing a credential shows no secret.
func (a APIToken) String() string {
	return fmt.Sprintf("APIToken{Email: %s, Token: %s}", a.Email, hidden)
}

// GoString is String, for %#v.
func (a APIToken) GoString() string {
	return a.String()
}

// LogValue logs the email address and hides the token.
func (a APIToken) LogValue() slog.Value {
	return slog.GroupValue(slog.String("email", a.Email), slog.String("token", hidden))
}

// String shows the key and hides the secret, so printing a credential shows no secret.
func (o OAuthConsumer) String() string {
	return fmt.Sprintf("OAuthConsumer{Key: %s, Secret: %s}", o.Key, hidden)
}

// GoString is String, for %#v.
func (o OAuthConsumer) GoString() string {
	return o.String()
}

// LogValue logs the key and hides the secret.
func (o OAuthConsumer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("key", o.Key), slog.String("secret", hidden))
}

func (a APIToken) check() error {
	if a.Email == "" {
		return fmt.Errorf("%w: the API token's email address is empty", ErrInvalidConfig)
	}
	if a.Token == "" {
		return fmt.Errorf("%w: the API token itself is empty", ErrInvalidConfig)
	}

	return nil
}

func (a APIToken) newLogin(login loginSettings) http.RoundTripper {
	return &basicLogin{
		next:     login.next,
		endpoint: login.endpoint,
		email:    a.Email,
		token:    a.Token,
	}
}

func (o OAuthConsumer) check() error {
	if o.Key == "" {
		return fmt.Errorf("%w: the OAuth consumer's key is empty", ErrInvalidConfig)
	}
	if o.Secret == "" {
		return fmt.Errorf("%w: the OAuth consumer's secret is empty", ErrInvalidConfig)
	}

	return nil
}

func (o OAuthConsumer) newLogin(login loginSettings) http.RoundTripper {
	return &bearerLogin{
		next:     login.next,
		endpoint: login.endpoint,
		tokens: &tokenSource{
			transport: login.tokenTransport,
			tokenURL:  login.tokenURL,
			key:       o.Key,
			secret:    o.Secret,
			now:       time.Now,
			lock:      make(chan struct{}, 1),
		},
	}
}
