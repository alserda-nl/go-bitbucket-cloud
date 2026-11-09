package models_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

func TestProject_DecodingKeepsEveryRecordedField(t *testing.T) {
	sample := readSample(t, "project.json")

	var project models.Project
	require.NoError(t, json.Unmarshal(sample, &project))

	assert.Empty(t, missingFields(t, sample, &project), "decoding project.json")
}

// TestProject_EachFieldDecodesFromItsOwnKey decodes a document whose values all differ, so a field
// whose JSON tag names a neighbour's key fails; the recorded sample cannot show that, as its
// workspace's name and slug are the same placeholder.
func TestProject_EachFieldDecodesFromItsOwnKey(t *testing.T) {
	document := `{
		"created_on": "2026-10-14T06:38:37Z",
		"description": "the description",
		"has_publicly_visible_repos": true,
		"is_private": false,
		"key": "KEY",
		"links": {"avatar": {"href": "avatar"}, "html": {"href": "html"}, "repositories": {"href": "repositories"}, "self": {"href": "self"}},
		"name": "the name",
		"owner": {
			"account_id": "account id",
			"display_name": "display name",
			"links": {"avatar": {"href": "owner avatar"}, "html": {"href": "owner html"}, "self": {"href": "owner self"}},
			"nickname": "nickname",
			"type": "team",
			"username": "username",
			"uuid": "{owner}"
		},
		"type": "project",
		"updated_on": "2026-10-15T07:00:00Z",
		"uuid": "{project}",
		"workspace": {"name": "workspace name", "slug": "workspace-slug", "type": "workspace", "uuid": "{workspace}"}
	}`
	expected := models.Project{
		CreatedOn:               time.Date(2026, 10, 14, 6, 38, 37, 0, time.UTC),
		Description:             "the description",
		HasPubliclyVisibleRepos: true,
		Key:                     "KEY",
		Links: models.ProjectLinks{
			Avatar:       models.Link{Href: "avatar"},
			HTML:         models.Link{Href: "html"},
			Repositories: models.Link{Href: "repositories"},
			Self:         models.Link{Href: "self"},
		},
		Name: "the name",
		Owner: models.Account{
			AccountID:   "account id",
			DisplayName: "display name",
			Links: models.AccountLinks{
				Avatar: models.Link{Href: "owner avatar"},
				HTML:   models.Link{Href: "owner html"},
				Self:   models.Link{Href: "owner self"},
			},
			Nickname: "nickname",
			Type:     "team",
			Username: "username",
			UUID:     "{owner}",
		},
		Type:      "project",
		UpdatedOn: time.Date(2026, 10, 15, 7, 0, 0, 0, time.UTC),
		UUID:      "{project}",
		Workspace: models.Workspace{
			Name: "workspace name",
			Slug: "workspace-slug",
			Type: "workspace",
			UUID: "{workspace}",
		},
	}

	var actual models.Project
	require.NoError(t, json.Unmarshal([]byte(document), &actual))

	assert.Equal(t, expected, actual, "json.Unmarshal should fill each field of a project from its own key")
}

func TestProject_TimestampsTakeAnyNumberOfFractionDigits(t *testing.T) {
	// A runner's create answers in nanoseconds where a read answers in microseconds.
	cases := []struct {
		name      string
		createdOn string
		expected  time.Time
	}{
		{
			name:      "microseconds, as a read answers",
			createdOn: "2026-10-14T06:38:37.737143+00:00",
			expected:  time.Date(2026, 10, 14, 6, 38, 37, 737143000, time.UTC),
		},
		{
			name:      "nanoseconds, as a runner's create answers",
			createdOn: "2026-10-25T09:12:01.123456789+00:00",
			expected:  time.Date(2026, 10, 25, 9, 12, 1, 123456789, time.UTC),
		},
		{
			name:      "no fraction",
			createdOn: "2026-10-14T06:38:37+00:00",
			expected:  time.Date(2026, 10, 14, 6, 38, 37, 0, time.UTC),
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var project models.Project
			require.NoError(t, json.Unmarshal(fmt.Appendf(nil, `{"created_on": %q}`, testCase.createdOn), &project))

			assert.True(t, testCase.expected.Equal(project.CreatedOn), "json.Unmarshal of created_on %q: want %s, got %s", testCase.createdOn, testCase.expected, project.CreatedOn)
		})
	}
}
