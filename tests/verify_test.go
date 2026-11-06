package e2e

import "testing"

func TestVerify_ANewProjectReadsBackOnceBitbucketShowsIt(t *testing.T) {
	// A new project answered 404 for at least 38 seconds on 2026-10-26.
	given, when, then := newVerifyStage(t)

	given.
		a_new_project_that_answers_404_for_reads("NEW", 2)

	when.
		the_project_is_verified("NEW")

	then.
		the_project_read_back().
		and().the_project_was_read_times("NEW", 3)
}

func TestVerify_AProjectThatNeverShowsIsNotVerified(t *testing.T) {
	given, when, then := newVerifyStage(t)

	given.
		a_new_project_that_answers_404_for_reads("NEW", 10).
		and().verify_reads_at_most(3)

	when.
		the_project_is_verified("NEW")

	then.
		the_project_is_not_verified().
		and().the_project_was_read_times("NEW", 3)
}
