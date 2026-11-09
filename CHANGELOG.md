# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `client` package: `New` with an API token (`APIToken`), an OAuth consumer (`OAuthConsumer`, the client credentials grant) or no credential, and the options `WithEndpoint`, `WithOAuthTokenURL`, `WithHTTPClient`, `WithMaxRetries`, `WithRetryWait` and `WithLogger`.
- Retries for 429, 500, 502, 503 and 504, doubling from 200 milliseconds to 5 seconds with jitter and honouring `Retry-After`; a POST other than the OAuth token request is tried again after a 429 only.
- One `*APIError` for every answer of 400 or above, matching `ErrNotFound` for every 404, and `ErrLogin` when an OAuth consumer gets no access token.
- List paging with `WithPageSize`, `WithLimit` and `WithFullWalk`, and `Verify` with `WithInterval` and `WithMaxAttempts`, which reads until a write shows.
- The first calls, `Workspaces.Get`, `Workspaces.ListProjects` and `Projects.Get`, with their types in the `models` package.
- `fakebitbucket` package: an in-memory Bitbucket Cloud for tests, with its token address and the builders `NewWorkspace` and `NewProject`.
