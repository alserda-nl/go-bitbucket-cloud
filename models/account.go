package models

// Account is a user or a team as another object embeds it, such as a project's owner.
type Account struct {
	// AccountID is a user's Atlassian account ID; a team has none.
	AccountID string `json:"account_id"`
	// DisplayName is the account's name as Bitbucket shows it.
	DisplayName string `json:"display_name"`
	// Links are the account's picture, its page on bitbucket.org and its address in the API.
	Links AccountLinks `json:"links"`
	// Nickname is the name a user chose; it need not be unique, and a team has none.
	Nickname string `json:"nickname"`
	// Type is "team" or "user".
	Type string `json:"type"`
	// Username is a team's username, which need not be its workspace's slug; a user has none.
	Username string `json:"username"`
	// UUID is the account's UUID, which never changes.
	UUID UUID `json:"uuid"`
}

// AccountLinks are an account's "links": account.Links.HTML.Href is its page on bitbucket.org.
type AccountLinks struct {
	// Avatar is the address of the account's picture.
	Avatar Link `json:"avatar"`
	// HTML is the account's page on bitbucket.org.
	HTML Link `json:"html"`
	// Self is the account's address in the API.
	Self Link `json:"self"`
}
