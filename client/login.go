package client

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// loginSettings is what a Credential needs to make its transport.
type loginSettings struct {
	// next is the transport below the login.
	next http.RoundTripper
	// endpoint is the API's address: only a request to its scheme and host gets the credential.
	endpoint *url.URL
	// tokenURL is where an OAuth consumer asks for its access token.
	tokenURL string
	// tokenTransport sends the token request, with retries of its own.
	tokenTransport http.RoundTripper
}

// withoutCredentialKey marks a context whose request goes without a credential.
type withoutCredentialKey struct{}

// withoutCredential returns a context whose call goes without the credential, for the calls
// Bitbucket refuses with every API token but answers without one: the two webhook event lists and
// the Pipelines OpenID Connect configuration and keys.
func withoutCredential(ctx context.Context) context.Context {
	return context.WithValue(ctx, withoutCredentialKey{}, true)
}

// getsCredential reports whether a request carries the credential: only a request to the
// client's own endpoint, its scheme and host, so a redirect elsewhere never does (a step log
// redirects to a signed Amazon S3 address), and no call marked withoutCredential.
func getsCredential(req *http.Request, endpoint *url.URL) bool {
	if req.Context().Value(withoutCredentialKey{}) != nil {
		return false
	}

	return onEndpoint(req.URL, endpoint)
}

// onEndpoint reports whether address has the endpoint's scheme and host, the host in any case.
func onEndpoint(address, endpoint *url.URL) bool {
	return address.Scheme == endpoint.Scheme && strings.EqualFold(address.Host, endpoint.Host)
}

// basicLogin sends an API token as Basic authentication.
type basicLogin struct {
	// next sends the request on.
	next http.RoundTripper
	// endpoint is the API's address: only a request to it gets the credential.
	endpoint *url.URL
	// email is the email address of the token's Atlassian account.
	email string
	// token is the API token.
	token string
}

// RoundTrip adds the API token to a copy of a request that gets the credential, as
// http.RoundTripper asks, and sends any other request as it is.
func (l *basicLogin) RoundTrip(req *http.Request) (*http.Response, error) {
	if !getsCredential(req, l.endpoint) {
		return l.next.RoundTrip(req)
	}

	authenticated := req.Clone(req.Context())
	authenticated.SetBasicAuth(l.email, l.token)

	return l.next.RoundTrip(authenticated)
}
