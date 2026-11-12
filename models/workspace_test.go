package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

func TestWorkspace_DecodingKeepsEveryRecordedField(t *testing.T) {
	assert.Empty(t, fieldsLostDecoding[models.Workspace](t, "workspace.json"), "json.Unmarshal of workspace.json into a models.Workspace should keep every field")
}

// TestWorkspace_EachFieldDecodesFromItsOwnKey decodes documents whose values all differ, so a field
// whose JSON tag names a neighbour's key fails; the recorded sample cannot show that, as its name
// and slug are the same placeholder.
func TestWorkspace_EachFieldDecodesFromItsOwnKey(t *testing.T) {
	cases := []struct {
		name     string
		document string
		expected models.Workspace
	}{
		{
			name: "every field",
			document: `{
				"created_on": "2026-10-10T08:00:00Z",
				"forking_mode": "allow_forks",
				"is_personal": true,
				"is_privacy_enforced": false,
				"is_private": false,
				"links": {
					"avatar": {"href": "avatar"}, "hooks": {"href": "hooks"}, "html": {"href": "html"},
					"html_overview": {"href": "html overview"}, "members": {"href": "members"}, "owners": {"href": "owners"},
					"projects": {"href": "projects"}, "repositories": {"href": "repositories"}, "self": {"href": "self"},
					"snippets": {"href": "snippets"}
				},
				"name": "the name",
				"slug": "the-slug",
				"type": "workspace",
				"uuid": "{workspace}"
			}`,
			expected: models.Workspace{
				CreatedOn:   time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC),
				ForkingMode: models.ForkingModeAllowForks,
				IsPersonal:  true,
				Links: models.WorkspaceLinks{
					Avatar:       models.Link{Href: "avatar"},
					Hooks:        models.Link{Href: "hooks"},
					HTML:         models.Link{Href: "html"},
					HTMLOverview: models.Link{Href: "html overview"},
					Members:      models.Link{Href: "members"},
					Owners:       models.Link{Href: "owners"},
					Projects:     models.Link{Href: "projects"},
					Repositories: models.Link{Href: "repositories"},
					Self:         models.Link{Href: "self"},
					Snippets:     models.Link{Href: "snippets"},
				},
				Name: "the name",
				Slug: "the-slug",
				Type: "workspace",
				UUID: "{workspace}",
			},
		},
		{
			name:     "a workspace whose privacy is enforced",
			document: `{"is_personal": false, "is_privacy_enforced": true, "is_private": false}`,
			expected: models.Workspace{
				IsPrivacyEnforced: true,
			},
		},
		{
			name:     "a private workspace",
			document: `{"is_personal": false, "is_privacy_enforced": false, "is_private": true}`,
			expected: models.Workspace{
				IsPrivate: true,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var actual models.Workspace
			require.NoError(t, json.Unmarshal([]byte(testCase.document), &actual))

			assert.Equal(t, testCase.expected, actual, "json.Unmarshal should fill each field of %s from its own key", testCase.name)
		})
	}
}
