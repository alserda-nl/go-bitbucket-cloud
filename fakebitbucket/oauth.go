package fakebitbucket

import (
	_ "embed"
	"fmt"
	"net/http"
)

//go:embed samples/oauth-token.json
var tokenSample []byte

// oauthRefusal is a refusal of the token address, as RFC 6749, section 5.2, describes one;
// Bitbucket's own is not recorded.
type oauthRefusal struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ExpireTokens makes every access token the token address issued answer 401, as Bitbucket's do
// after an hour.
func (s *Server) ExpireTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens = map[string]bool{}
}

// handleToken answers the client credentials grant: a POST of grant_type=client_credentials with
// the consumer's key and secret as Basic authentication.
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	key, secret, ok := r.BasicAuth()
	if !ok || key != OAuthKey || secret != OAuthSecret {
		writeJSON(w, http.StatusUnauthorized, oauthRefusal{
			Error:            "invalid_client",
			ErrorDescription: "Invalid OAuth client credentials",
		})
		return
	}
	if r.PostFormValue("grant_type") != "client_credentials" {
		writeJSON(w, http.StatusBadRequest, oauthRefusal{
			Error:            "unsupported_grant_type",
			ErrorDescription: "The fake answers the client credentials grant only",
		})
		return
	}

	var answer map[string]any
	decodeSample(tokenSample, &answer)
	answer["access_token"] = s.issueToken()

	writeJSON(w, http.StatusOK, answer)
}

func (s *Server) issueToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.issued++
	token := fmt.Sprintf("fake-access-token-%d", s.issued)
	s.tokens[token] = true

	return token
}
