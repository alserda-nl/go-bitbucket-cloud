package e2e

import "testing"

func TestWorkspaces_ASlugReadsInAnyCase(t *testing.T) {
	given, when, then := newWorkspaceStage(t)

	given.a_private_workspace("acme")

	when.the_workspace_is_read("ACME")

	then.the_workspace_is_returned()
}

func TestWorkspaces_AUUIDInBracesReads(t *testing.T) {
	given, when, then := newWorkspaceStage(t)

	given.a_private_workspace("acme")

	when.the_workspace_is_read_by_its_uuid()

	then.the_workspace_is_returned()
}

func TestWorkspaces_APrivateWorkspaceReadsWithoutACredential(t *testing.T) {
	// Bitbucket answers even a private workspace's details without a credential.
	given, when, then := newWorkspaceStage(t)

	given.a_private_workspace("acme").
		and().the_client_logs_in_with(noCredential())

	when.the_workspace_is_read("acme")

	then.the_workspace_is_returned()
}

func TestWorkspaces_AnUnknownWorkspaceIsNotFound(t *testing.T) {
	_, when, then := newWorkspaceStage(t)

	when.the_workspace_is_read("nobody")

	then.the_workspace_is_not_found()
}
