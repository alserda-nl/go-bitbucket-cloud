# Sample answers

Each file here is an answer Bitbucket Cloud gave in the maintainers' test workspace, with every UUID and every name of an account or a workspace replaced by a placeholder, unless the table marks it as hand-written. The fake's builders start from them, so a test starts from what Bitbucket sends. The maintainers make these files, and this one, from the answers they record in that workspace: a sample changes through a new recording, not by hand.

| Sample | Endpoint | Replaced, or the source of a hand-written one |
|---|---|---|
| `errors/branch-exists.json` | `POST /2.0/repositories/{workspace}/{repo_slug}/refs/branches` | nothing |
| `errors/commit-not-found.json` | `POST /2.0/repositories/{workspace}/{repo_slug}/refs/branches` | nothing |
| `errors/file-not-found.json` | `GET /2.0/repositories/{workspace}/{repo_slug}/src/{commit}/{path}` | nothing |
| `errors/invalid-pagelen.json` | `GET /2.0/workspaces/{workspace}/projects` | nothing |
| `errors/parent-not-found.json` | `POST /2.0/repositories/{workspace}/{repo_slug}/src` | nothing |
| `errors/project-not-empty.json` | `DELETE /2.0/workspaces/{workspace}/projects/{project_key}` | nothing |
| `errors/project-not-found.json` | `GET /2.0/workspaces/{workspace}/projects/{project_key}` | nothing |
| `errors/runner-not-found.json` | `DELETE /2.0/workspaces/{workspace}/pipelines-config/runners/{runner_uuid}` | 1 UUID |
| `errors/schedule-server-error.json` | `POST /2.0/repositories/{workspace}/{repo_slug}/pipelines_config/schedules` | 1 error ID |
| `errors/scope-refused.json` | `GET /2.0/workspaces/{workspace}/projects/{project_key}` | nothing |
| `errors/server-error.json` | `POST /2.0/repositories/{workspace}/{repo_slug}/src` | 1 error ID |
| `errors/text-answer.json` | `POST /2.0/workspaces/{workspace}/pipelines-config/runners` | hand-written: Bitbucket answers some errors in text/plain or text/html: the text here is made up, the status and the content type are Bitbucket's |
| `errors/variable-duplicate.json` | `POST /2.0/repositories/{workspace}/{repo_slug}/deployments_config/environments/{environment_uuid}/variables` | 2 UUIDs |
| `oauth-token.json` | `POST https://bitbucket.org/site/oauth2/access_token` | hand-written: Atlassian's OAuth 2.0 documentation and changelog: one hour, no refresh token for the client credentials grant and `scope` instead of `scopes` since 2026-05-04 |
| `project.json` | `GET /2.0/workspaces/{workspace}/projects/{project_key}` | 2 UUIDs, 2 names, 1 description, 1 project name |
| `workspace.json` | `GET /2.0/workspaces/{workspace}` | 1 UUID, 1 name |
