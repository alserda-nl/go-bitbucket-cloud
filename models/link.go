package models

// Link is one of the addresses in an object's links.
type Link struct {
	// Href is the address itself, such as https://api.bitbucket.org/2.0/workspaces/acme.
	Href string `json:"href"`
}
