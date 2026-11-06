package models_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alserda-nl/go-bitbucket-cloud/models"
)

func TestModels_DecodingKeepsEveryRecordedField(t *testing.T) {
	cases := []struct {
		name    string
		sample  string
		decoded any
	}{
		{
			name:    "a workspace",
			sample:  "workspace.json",
			decoded: &models.Workspace{},
		},
		{
			name:    "a project",
			sample:  "project.json",
			decoded: &models.Project{},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			sample := readSample(t, testCase.sample)

			require.NoError(t, json.Unmarshal(sample, testCase.decoded))

			assert.Empty(t, missingFields(t, sample, testCase.decoded), "decoding %s", testCase.sample)
		})
	}
}

// TestModels_EachFieldDecodesFromItsOwnKey decodes documents whose values all differ, so a field
// whose JSON tag names a neighbour's key fails; the recorded samples cannot show that, as their
// workspace's name and slug are the same placeholder.
func TestModels_EachFieldDecodesFromItsOwnKey(t *testing.T) {
	cases := []struct {
		name     string
		document string
		decoded  any
		expected any
	}{
		{
			name: "a project",
			document: `{
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
			}`,
			decoded: &models.Project{},
			expected: &models.Project{
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
			},
		},
		{
			name: "a workspace",
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
			decoded: &models.Workspace{},
			expected: &models.Workspace{
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
			decoded:  &models.Workspace{},
			expected: &models.Workspace{
				IsPrivacyEnforced: true,
			},
		},
		{
			name:     "a private workspace",
			document: `{"is_personal": false, "is_privacy_enforced": false, "is_private": true}`,
			decoded:  &models.Workspace{},
			expected: &models.Workspace{
				IsPrivate: true,
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			require.NoError(t, json.Unmarshal([]byte(testCase.document), testCase.decoded))

			assert.Equal(t, testCase.expected, testCase.decoded, "json.Unmarshal should fill each field of %s from its own key", testCase.name)
		})
	}
}

func TestProject_TimestampsTakeAnyNumberOfFractionDigits(t *testing.T) {
	// A runner's create answers in nanoseconds where its read has microseconds (2026-10-25).
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

			assert.True(t, testCase.expected.Equal(project.CreatedOn), "created_on %q: want %s, got %s", testCase.createdOn, testCase.expected, project.CreatedOn)
		})
	}
}

// readSample reads one of the fake's samples: real answers, with names and IDs replaced.
func readSample(t *testing.T, name string) []byte {
	t.Helper()

	sample, err := os.ReadFile(fmt.Sprintf("../fakebitbucket/samples/%s", name))
	require.NoError(t, err)

	return sample
}

// missingFields encodes decoded again and returns every field of sample it lost, or holds as
// another JSON type. Fields only decoded has are fine: a short form leaves fields zero.
func missingFields(t *testing.T, sample []byte, decoded any) []string {
	t.Helper()

	encoded, err := json.Marshal(decoded)
	require.NoError(t, err)

	var expected, actual any
	require.NoError(t, json.Unmarshal(sample, &expected))
	require.NoError(t, json.Unmarshal(encoded, &actual))

	return compareFields("", expected, actual)
}

func compareFields(path string, expected, actual any) []string {
	switch expectedValue := expected.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: want an object, got %T", path, actual)}
		}

		var missing []string
		for key, item := range expectedValue {
			if _, found := actualValue[key]; !found {
				missing = append(missing, fmt.Sprintf("%s.%s: missing", path, key))
				continue
			}

			missing = append(missing, compareFields(fmt.Sprintf("%s.%s", path, key), item, actualValue[key])...)
		}

		return missing
	case nil:
		return nil
	default:
		if fmt.Sprintf("%T", expected) != fmt.Sprintf("%T", actual) {
			return []string{fmt.Sprintf("%s: want %T, got %T", path, expected, actual)}
		}

		return nil
	}
}
