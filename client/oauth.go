package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// bearerLogin sends an OAuth consumer's access token as a Bearer token.
type bearerLogin struct {
	next     http.RoundTripper
	endpoint *url.URL
	tokens   *tokenSource
}

// RoundTrip sends the request with the current access token. A token Bitbucket no longer takes
// answers 401 (Atlassian's OAuth documentation): it asks for a new one once and sends the request again.
func (l *bearerLogin) RoundTrip(req *http.Request) (*http.Response, error) {
	if !getsCredential(req, l.endpoint) {
		return l.next.RoundTrip(req)
	}

	token, err := l.tokens.current(req.Context())
	if err != nil {
		closeBody(req)
		return nil, err
	}

	response, err := l.next.RoundTrip(withBearer(req, token))
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		return response, err
	}

	// A body that cannot be sent again leaves the 401 as the answer.
	again, err := rewindRequest(req)
	if err != nil {
		return response, nil
	}

	drainAndClose(response.Body)
	fresh, err := l.tokens.renew(req.Context(), token)
	if err != nil {
		closeBody(again)
		return nil, err
	}

	return l.next.RoundTrip(withBearer(again, fresh))
}

func withBearer(req *http.Request, token string) *http.Request {
	authenticated := req.Clone(req.Context())
	authenticated.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	return authenticated
}

// tokenSource asks the token address for access tokens with the client credentials grant and
// keeps the current one until its expires_in has passed. The grant gives no refresh token since
// 2026-05-04, so a new token is a new grant.
type tokenSource struct {
	// transport sends the token request without an http.Client, which would follow a redirect with
	// the consumer's key and secret still in its header.
	transport http.RoundTripper
	tokenURL  string
	key       string
	secret    string
	now       func() time.Time

	// lock holds a value while a token is fetched, so requests that need one wait for the same
	// fetch; a request whose context ends stops waiting.
	lock      chan struct{}
	token     string
	expiresAt time.Time
}

// current returns the token, asking for one when there is none or the current one has expired.
func (s *tokenSource) current(ctx context.Context) (string, error) {
	return s.whileLocked(ctx, func() (string, error) {
		if s.token != "" && s.now().Before(s.expiresAt) {
			return s.token, nil
		}

		return s.fetch(ctx)
	})
}

// renew returns a token other than stale, the one a 401 refused. When another request renewed it
// in the meantime it returns that one instead of asking again: requests that were refused together
// otherwise each fetch a token, one after the other.
func (s *tokenSource) renew(ctx context.Context, stale string) (string, error) {
	return s.whileLocked(ctx, func() (string, error) {
		if s.token != stale {
			return s.token, nil
		}

		return s.fetch(ctx)
	})
}

// whileLocked returns what get returns with the lock held, or gives up when ctx ends before the
// lock is free.
func (s *tokenSource) whileLocked(ctx context.Context, get func() (string, error)) (string, error) {
	select {
	case s.lock <- struct{}{}:
	case <-ctx.Done():
		return "", fmt.Errorf("waiting for another request's access token: %w", context.Cause(ctx))
	}
	defer func() { <-s.lock }()

	return get()
}

// tokenAnswer is the token address's answer; "scope" and "token_type" are not used.
type tokenAnswer struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// fetch asks for a new token; the lock is held.
func (s *tokenSource) fetch(ctx context.Context) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrLogin, err)
	}

	request.SetBasicAuth(s.key, s.secret)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", jsonMediaType)

	response, err := s.transport.RoundTrip(request)
	if err != nil {
		return "", fmt.Errorf("%w: asking %s for an access token: %w", ErrLogin, s.tokenURL, err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("%w: reading the access token from %s: %w", ErrLogin, s.tokenURL, err)
	}
	if response.StatusCode != http.StatusOK {
		refusal := newAPIError(http.MethodPost, request.URL.EscapedPath(), response.StatusCode, response.Header.Get("Content-Type"), body)
		// As text, not as an *APIError: its 404 must not match ErrNotFound for the object a call asked for.
		return "", fmt.Errorf("%w: %s", ErrLogin, refusal.describe())
	}

	var answer tokenAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", fmt.Errorf("%w: decoding the token address's answer: %w", ErrLogin, err)
	}
	if answer.AccessToken == "" {
		return "", fmt.Errorf("%w: the token address answered without an access token", ErrLogin)
	}

	s.token = answer.AccessToken
	s.expiresAt = s.now().Add(time.Duration(answer.ExpiresIn) * time.Second)

	return s.token, nil
}
