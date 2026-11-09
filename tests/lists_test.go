package e2e

import (
	"testing"

	"github.com/alserda-nl/go-bitbucket-cloud/client"
)

func TestListProjects_FetchesEveryPageOfTheLargestSize(t *testing.T) {
	given, when, then := newListStage(t)

	given.projects_in_the_workspace(250)

	when.the_projects_of_are_listed("acme")

	then.projects_are_returned(250).
		and().the_pages_asked_for_were("100", "100", "100")
}

func TestListProjects_ALimitAsksForOneSmallPage(t *testing.T) {
	given, when, then := newListStage(t)

	given.projects_in_the_workspace(250).
		and().the_list_options_are(client.WithLimit(5))

	when.the_projects_of_are_listed("acme")

	then.projects_are_returned(5).
		and().the_pages_asked_for_were("5")
}

func TestListProjects_ASmallerPageSizeAsksForMorePages(t *testing.T) {
	given, when, then := newListStage(t)

	given.projects_in_the_workspace(25).
		and().the_list_options_are(client.WithPageSize(10))

	when.the_projects_of_are_listed("acme")

	then.projects_are_returned(25).
		and().the_pages_asked_for_were("10", "10", "10")
}

func TestListProjects_APageSizeAboveTheLargestIsRefused(t *testing.T) {
	// Bitbucket refuses a page size of 101 with 400 "Invalid pagelen".
	given, when, then := newListStage(t)

	given.the_list_options_are(client.WithPageSize(101))

	when.the_projects_of_are_listed("acme")

	then.the_option_is_refused_before_any_request()
}

func TestListProjects_AnUnknownWorkspaceIsNotFound(t *testing.T) {
	_, when, then := newListStage(t)

	when.the_projects_of_are_listed("nobody")

	then.the_workspace_is_not_found()
}
