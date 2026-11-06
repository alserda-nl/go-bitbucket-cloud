package e2e

import (
	"net/http"
	"testing"

	"github.com/alserda-nl/go-bitbucket-cloud/fakebitbucket"
)

func TestLogin_AnAPITokenReadsAPrivateProject(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_logs_in_with(apiToken())

	when.
		the_project_is_read("ABC")

	then.
		the_project_is_returned("ABC").
		and().every_api_request_carried(fakebitbucket.CredentialBasic).
		and().the_token_address_was_asked(0)
}

func TestLogin_AnOAuthConsumerReadsWithAnAccessToken(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_logs_in_with(oauthConsumer())

	when.
		the_project_is_read("ABC").
		and().the_project_is_read("ABC")

	then.
		the_project_is_returned("ABC").
		and().every_api_request_carried(fakebitbucket.CredentialBearer).
		and().the_token_address_was_asked(1)
}

func TestLogin_AnExpiredAccessTokenIsRenewedOnce(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_logs_in_with(oauthConsumer())

	when.
		the_project_is_read("ABC")

	// Bitbucket's access tokens answer 401 after an hour.
	given.
		the_access_token_has_expired()

	when.
		the_project_is_read("ABC")

	then.
		the_project_is_returned("ABC").
		and().the_token_address_was_asked(2)
}

func TestLogin_TheLogShowsTheTokenRequestsAndTheReadSentAgain(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_logs_in_with_a_log(oauthConsumer())

	when.
		the_project_is_read("ABC")

	// The expired token's 401, the new token and the read sent again each log a line.
	given.
		the_access_token_has_expired()

	when.
		the_project_is_read("ABC")

	then.
		the_project_is_returned("ABC").
		and().the_log_has_a_line_per_attempt(
		"method=POST path=/site/oauth2/access_token",
		"method=GET path=/2.0/workspaces/acme/projects/ABC",
		"method=GET path=/2.0/workspaces/acme/projects/ABC",
		"method=POST path=/site/oauth2/access_token",
		"method=GET path=/2.0/workspaces/acme/projects/ABC",
	)
}

func TestLogin_ARefusedAPITokenGets401(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().an_api_token_bitbucket_does_not_take()

	when.
		the_project_is_read("ABC")

	then.
		the_credential_is_refused()
}

func TestLogin_RequestsRefusedTogetherRenewTheTokenOnce(t *testing.T) {
	// Twenty reads whose token expired at once: one renewal serves them all.
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_logs_in_with(oauthConsumer())

	when.
		the_project_is_read("ABC")

	given.
		the_access_token_has_expired()

	when.
		the_project_is_read_by_requests_at_once("ABC", 20)

	then.
		every_read_succeeded().
		and().the_token_address_was_asked(2)
}

func TestLogin_ATokenRequestAnswered429IsTriedAgain(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_logs_in_with(oauthConsumer()).
		and().bitbucket_answers_the_next_request_with(http.StatusTooManyRequests)

	when.
		the_project_is_read("ABC")

	then.
		the_project_is_returned("ABC").
		and().the_token_address_was_asked(2)
}

func TestLogin_ATokenRequestAnswered503IsTriedAgain(t *testing.T) {
	given, when, then := newLoginStage(t)

	// Unlike a call's POST: a second grant only issues another token.
	given.
		a_private_project("ABC").
		and().the_client_logs_in_with(oauthConsumer()).
		and().bitbucket_answers_the_next_request_with(http.StatusServiceUnavailable)

	when.
		the_project_is_read("ABC")

	then.
		the_project_is_returned("ABC").
		and().the_token_address_was_asked(2)
}

func TestLogin_AWrongTokenAddressFailsTheLoginNotTheRead(t *testing.T) {
	// Whatever a wrong token address answers, here the fake's 401 for a credential it takes only at
	// the token address, the login fails, never the read: a provider drops a resource from state on
	// a 404.
	given, when, then := newLoginStage(t)

	given.
		a_private_project("ABC").
		and().the_client_asks_for_its_token_at("/site/oauth2/no-such-address")

	when.
		the_project_is_read("ABC")

	then.
		the_login_failed_without_reading_as_a_missing_project()
}

func TestLogin_WithoutACredentialOnlyPublicProjectsShow(t *testing.T) {
	given, when, then := newLoginStage(t)

	given.
		a_private_project("PRIVATE").
		and().a_public_project("PUBLIC").
		and().the_client_logs_in_with(noCredential())

	when.
		the_projects_are_listed()

	then.
		only_these_projects_are_listed("PUBLIC")

	// Reading the private one answers 404, not 401 (2026-10-30).
	when.
		the_project_is_read("PRIVATE")

	then.
		the_project_is_not_found()
}
