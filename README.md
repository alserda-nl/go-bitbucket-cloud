# go-bitbucket-cloud

go-bitbucket-cloud is a Go client for the [Bitbucket Cloud REST API](https://developer.atlassian.com/cloud/bitbucket/rest/intro/). It is for Bitbucket Cloud only, not Bitbucket Data Center or Server. Its runtime code depends on the Go standard library only.

The module also ships `fakebitbucket`, an in-memory Bitbucket for tests, built from answers Bitbucket Cloud really gave.

## Install

```sh
go get github.com/alserda-nl/go-bitbucket-cloud
```

## Usage

```go
apiClient, err := client.New(client.APIToken{
	Email: "someone@example.com",
	Token: os.Getenv("BITBUCKET_API_TOKEN"),
})
if err != nil {
	log.Fatal(err)
}

project, err := apiClient.Projects.Get(ctx, "my-workspace", "PROJ")
if errors.Is(err, client.ErrNotFound) {
	// no such project, or the credential may not see it
}
```

`client.New` takes one credential: an Atlassian account's API token, sent as Basic authentication, or a workspace's OAuth consumer (`client.OAuthConsumer{Key: ..., Secret: ...}`), which the client trades for an access token with the client credentials grant and renews when it expires. A `nil` credential sends none, and Bitbucket then shows only what anyone may see.

### Options

```go
client.New(credential,
	client.WithEndpoint("https://api.bitbucket.org"),                         // default shown
	client.WithOAuthTokenURL("https://bitbucket.org/site/oauth2/access_token"), // default shown
	client.WithHTTPClient(customHTTPClient),
	client.WithMaxRetries(3),                   // default shown
	client.WithRetryWait(200*time.Millisecond), // default shown
	client.WithLogger(logger),                  // off by default
)
```

An answer of 429, 500, 502, 503 or 504 is tried again, waiting 200 milliseconds and doubling up to 5 seconds, plus or minus 20 percent; a `Retry-After` header wins when Bitbucket sends one. A POST is tried again after a 429 only: after a 5xx Bitbucket may have done the work, and a second POST could, for example, start a pipeline twice. The OAuth consumer's token request is the exception and is tried again after a 5xx too, since a second request only gets another token.

The logger gets one line per attempt, the token request's included: the method, the path, the status, the attempt's number and how long it took until the answer's headers. Never the query, a header or a body, which can hold a signed address, a variable's value or the credential. An attempt that got no answer, a 429 or a 5xx logs at warn level, every other at debug level, so give the logger a handler that shows the level you want:

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
```

### Lists

A list call fetches every page unless an option says otherwise, asking for the largest page the list takes:

```go
projects, err := apiClient.Workspaces.ListProjects(ctx, "my-workspace")                      // every project
first, err := apiClient.Workspaces.ListProjects(ctx, "my-workspace", client.WithLimit(5))     // one request
pages, err := apiClient.Workspaces.ListProjects(ctx, "my-workspace", client.WithPageSize(10)) // smaller pages
```

### Errors

Every answer of 400 or above comes back as a `*client.APIError`, whatever the shape Bitbucket answered in: its status, message, detail, the fields it refused, the key and arguments of the Pipelines services, on a 403 for a scope, the scopes the call needs and the ones the credential has, and on a 500 the error ID Atlassian's support asks for. Its text also says how often the call was sent when the retries ran out. `errors.Is(err, client.ErrNotFound)` is true for every 404. When an OAuth consumer gets no access token, the call fails with `client.ErrLogin` instead, which never matches `ErrNotFound`.

### Waiting for a write to show

Bitbucket does not always answer with what was just written: a new project can answer 404 for 38 seconds and more. `client.Verify` reads until a check says the object is there as expected, 3 seconds apart and at most 20 times by default. A 404, a 429, a 5xx or a read that got no answer reads again; any other refusal ends the reads at once. However the reads end, the error matches `client.ErrNotVerified`, as the object was written:

```go
err := client.Verify(ctx, func(ctx context.Context) (bool, error) {
	project, err := apiClient.Projects.Get(ctx, "my-workspace", "PROJ")
	if err != nil {
		return false, err
	}
	if project.Name != "Wanted name" {
		return false, fmt.Errorf("name is %q, want %q", project.Name, "Wanted name")
	}

	return true, nil
})
if errors.Is(err, client.ErrNotVerified) {
	// written, but not read back as expected in time
}
```

## Testing against the fake

```go
func TestSomething(t *testing.T) {
	fake := fakebitbucket.New(t)
	workspace := fakebitbucket.NewWorkspace(fakebitbucket.Workspace.WithSlug("acme"))
	fake.SeedWorkspace(workspace)
	fake.SeedProject(fakebitbucket.NewProject(fakebitbucket.Project.WithWorkspace(workspace), fakebitbucket.Project.WithKey("ABC")))

	apiClient, err := client.New(
		client.APIToken{Email: fakebitbucket.Email, Token: fakebitbucket.APIToken},
		client.WithEndpoint(fake.URL),
	)
	// ...
}
```

The fake answers as Bitbucket does, from answers Bitbucket gave (`fakebitbucket/samples/README.md` lists them), and its package documentation lists where it knowingly differs.

## Development

```sh
make lint   # golangci-lint, the version pinned in the Makefile
make test   # every test, with the race detector
make cover  # every test, and how much of the code they cover
```

## Versioning

Releases follow [semantic versioning](https://semver.org). Every merge to `main` tags and releases a version: the [Conventional Commits](https://www.conventionalcommits.org) type of the merged commit picks the bump, and a commit without one releases a patch. Until v1.0.0, a minor release may break the API; the [changelog](CHANGELOG.md) says where.

## Contributing

Issues and pull requests are welcome. Run `make lint` and `make cover` before opening one, and write the commit message as a Conventional Commit, since it picks the next version.

## License

MIT
