package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// A Credential logs a Client in to Bitbucket: an APIToken or an OAuthConsumer. A nil Credential
// sends none, and Bitbucket then answers as to anyone: it shows a private workspace's details,
// lists only its public projects and answers 404 for a private project.
type Credential interface {
	// check returns ErrInvalidConfig when a part of the credential is missing.
	check() error
	// newLogin returns the transport that adds the credential to a request.
	newLogin(login loginSettings) http.RoundTripper
}

// APIToken is an Atlassian account's API token, sent with the account's email address as Basic
// authentication. Printing it with fmt, logging it with slog or encoding it as JSON hides the
// token; a printer that reads the fields by reflection, such as a test's diff, still shows it.
type APIToken struct {
	// Email is the email address of the Atlassian account the token belongs to.
	Email string
	// Token is the API token itself.
	Token string
}

// OAuthConsumer is a workspace's OAuth consumer: the Client trades its key and secret for an
// access token with the client credentials grant, and asks for a new token when the old one
// expires or a call answers 401. Printing it with fmt, logging it with slog or encoding it as JSON
// hides the secret; a printer that reads the fields by reflection, such as a test's diff, still
// shows it.
type OAuthConsumer struct {
	// Key is the consumer's key, from the workspace's OAuth consumer settings.
	Key string
	// Secret is the consumer's secret, from the same settings.
	Secret string
}

// hidden stands in for a token or a secret wherever a credential is printed, logged or encoded.
const hidden = "(hidden)"

// String shows the email address and hides the token, so printing a credential shows no secret:
// APIToken{Email: someone@example.com, Token: (hidden)}.
func (a APIToken) String() string {
	return fmt.Sprintf("APIToken{Email: %s, Token: %s}", a.Email, hidden)
}

// Format writes String's text for every fmt verb, so %d, %x and %#v hide the token too.
func (a APIToken) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, a.String())
}

// MarshalJSON encodes the API token with the token hidden:
// {"Email":"someone@example.com","Token":"(hidden)"}.
func (a APIToken) MarshalJSON() ([]byte, error) {
	type withoutMethods APIToken

	return json.Marshal(withoutMethods{Email: a.Email, Token: hidden})
}

// LogValue makes slog log the email address and hide the token.
func (a APIToken) LogValue() slog.Value {
	return slog.GroupValue(slog.String("email", a.Email), slog.String("token", hidden))
}

// String shows the key and hides the secret, so printing a credential shows no secret:
// OAuthConsumer{Key: abc, Secret: (hidden)}.
func (o OAuthConsumer) String() string {
	return fmt.Sprintf("OAuthConsumer{Key: %s, Secret: %s}", o.Key, hidden)
}

// Format writes String's text for every fmt verb, so %d, %x and %#v hide the secret too.
func (o OAuthConsumer) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, o.String())
}

// MarshalJSON encodes the OAuth consumer with the secret hidden: {"Key":"abc","Secret":"(hidden)"}.
func (o OAuthConsumer) MarshalJSON() ([]byte, error) {
	type withoutMethods OAuthConsumer

	return json.Marshal(withoutMethods{Key: o.Key, Secret: hidden})
}

// LogValue makes slog log the key and hide the secret.
func (o OAuthConsumer) LogValue() slog.Value {
	return slog.GroupValue(slog.String("key", o.Key), slog.String("secret", hidden))
}

// nilPointer reports whether credential is a nil *APIToken or *OAuthConsumer: a Credential that
// is not nil itself but holds none.
func nilPointer(credential Credential) bool {
	switch pointer := credential.(type) {
	case *APIToken:
		return pointer == nil
	case *OAuthConsumer:
		return pointer == nil
	default:
		return false
	}
}

// check refuses an API token without an email address or without the token itself.
func (a APIToken) check() error {
	if a.Email == "" {
		return fmt.Errorf("%w: the API token's email address is empty", ErrInvalidConfig)
	}
	if a.Token == "" {
		return fmt.Errorf("%w: the API token itself is empty", ErrInvalidConfig)
	}

	return nil
}

// newLogin returns the transport that sends the token as Basic authentication.
func (a APIToken) newLogin(login loginSettings) http.RoundTripper {
	return &basicLogin{
		next:     login.next,
		endpoint: login.endpoint,
		email:    a.Email,
		token:    a.Token,
	}
}

// check refuses an OAuth consumer without a key or without a secret.
func (o OAuthConsumer) check() error {
	if o.Key == "" {
		return fmt.Errorf("%w: the OAuth consumer's key is empty", ErrInvalidConfig)
	}
	if o.Secret == "" {
		return fmt.Errorf("%w: the OAuth consumer's secret is empty", ErrInvalidConfig)
	}

	return nil
}

// newLogin returns the transport that sends an access token, asked for at login's token address.
func (o OAuthConsumer) newLogin(login loginSettings) http.RoundTripper {
	return &bearerLogin{
		next:     login.next,
		endpoint: login.endpoint,
		tokens:   newTokenSource(login.tokenTransport, login.tokenURL, o.Key, o.Secret),
	}
}
