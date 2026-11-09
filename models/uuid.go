package models

// UUID is the UUID Bitbucket gives an object, in braces as Bitbucket writes it:
// {00000000-0000-4000-8000-000000000001}. A path takes a workspace's, repository's or account's
// UUID in braces where it takes its slug or account ID; a workspace's UUID without the braces finds
// nothing. UUID is another name for string, so a call takes either, and the client escapes the
// braces in a path.
type UUID = string
