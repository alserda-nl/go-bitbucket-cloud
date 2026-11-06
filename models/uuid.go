package models

// UUID is the UUID Bitbucket gives an object, written as Bitbucket writes it: in braces, as in
// {00000000-0000-4000-8000-000000000001}. Bitbucket wants the braces back in a path: its
// specification lets a path name a workspace, repository, project or account by its UUID in
// braces where it takes the slug, the key or the account ID, and a workspace's UUID without
// braces found nothing (2026-10-19). UUID is another name for string, so a call that takes a slug
// or a UUID takes either; the client escapes the braces in a path.
type UUID = string
