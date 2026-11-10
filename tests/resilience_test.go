package e2e

import (
	"net/http"
	"testing"
)

func TestResilience_ServerErrorsAreTriedAgain(t *testing.T) {
	// Three 503s with the default three retries: attempts 1 to 3 fail, attempt 4 succeeds.
	given, when, then := newResilienceStage(t)

	given.a_project("ABC").
		and().bitbucket_answers_with_status_n_times(http.StatusServiceUnavailable, 3)

	when.the_project_is_read("ABC")

	then.the_project_is_returned("ABC").
		and().the_project_was_requested_n_times(4)
}

func TestResilience_RateLimitIsTriedAgain(t *testing.T) {
	given, when, then := newResilienceStage(t)

	given.a_project("ABC").
		and().bitbucket_answers_with_status_n_times(http.StatusTooManyRequests, 2)

	when.the_project_is_read("ABC")

	then.the_project_is_returned("ABC").
		and().the_project_was_requested_n_times(3)
}

func TestResilience_ExhaustedRetriesReturnBitbucketsAnswer(t *testing.T) {
	given, when, then := newResilienceStage(t)

	given.a_project("ABC").
		and().the_client_tries_again_at_most(2).
		and().bitbucket_answers_with_status_n_times(http.StatusServiceUnavailable, 3)

	when.the_project_is_read("ABC")

	then.bitbucket_answer_is_returned(http.StatusServiceUnavailable).
		and().the_project_was_requested_n_times(3)
}

func TestResilience_NotFoundIsNotTriedAgain(t *testing.T) {
	given, when, then := newResilienceStage(t)

	given.the_client_tries_again_at_most(3)

	when.the_project_is_read("ABC")

	then.bitbucket_answer_is_returned(http.StatusNotFound).
		and().the_project_was_requested_n_times(1)
}

func TestResilience_EveryAttemptIsLogged(t *testing.T) {
	given, when, then := newResilienceStage(t)

	given.a_project("ABC").
		and().bitbucket_answers_with_status_n_times(http.StatusServiceUnavailable, 1)

	when.the_project_is_read("ABC")

	then.the_log_has_a_line_per_attempt(
		"method=GET path=/2.0/workspaces/acme/projects/ABC attempt=1",
		"method=GET path=/2.0/workspaces/acme/projects/ABC attempt=2",
	)
}
