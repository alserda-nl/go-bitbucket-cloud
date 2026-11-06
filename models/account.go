package models

// Account is a user or a team, as Bitbucket embeds one in another object: a project's owner is
// its workspace's team, which has a username (2026-10-14); a user has an account ID and a
// nickname instead (2026-10-16).
type Account struct {
	AccountID   string       `json:"account_id"`
	DisplayName string       `json:"display_name"`
	Links       AccountLinks `json:"links"`
	Nickname    string       `json:"nickname"`
	// Type is "team" or "user".
	Type     string `json:"type"`
	Username string `json:"username"`
	UUID     UUID   `json:"uuid"`
}

// AccountLinks are an account's links.
type AccountLinks struct {
	Avatar Link `json:"avatar"`
	HTML   Link `json:"html"`
	Self   Link `json:"self"`
}
