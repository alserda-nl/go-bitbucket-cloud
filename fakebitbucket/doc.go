// Package fakebitbucket is an in-memory Bitbucket Cloud for tests: this module's tests and a
// Terraform provider's run against it in-process, with no network and no real credential. It
// depends on the standard library and net/http/httptest only, so importing it adds no test library
// to a build.
//
// It answers as Bitbucket does, not as a client sends: its routes, statuses, error answers and the
// forms it stores come from answers Bitbucket gave, and each builder starts from
// one of those answers with every name and ID replaced (samples/README.md lists them).
//
// Where it knowingly differs from Bitbucket:
//   - A timestamp ends in "Z" where Bitbucket writes "+00:00". A project without a description has
//     "", where Bitbucket answers null for a description that was never set.
//   - A refused credential answers 401 in the repository endpoints' error shape; Bitbucket's own
//     answer to one is not recorded. The token address answers a refused consumer as RFC 6749,
//     section 5.2, describes.
//   - A list page has no "previous" link, which Bitbucket gives from the second page on, and an
//     avatar link has no "?ts=" query.
//   - A list or a project in a workspace that does not exist answers the workspace's 404, which is
//     not recorded either.
//   - A path it has no route for answers 501, so a call to a wrong address fails instead of looking
//     like a 404.
package fakebitbucket
