package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestCandidateTagsAndSkills(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	siti := a.createCandidate(token, map[string]any{
		"firstName": "Siti", "industry": "Banking", "tags": []string{" Relocate ", "relocate", "Hot"}, "skills": []string{"IFRS", "  Excel  "},
	})
	assert.Equal(t, []string{"Relocate", "Hot"}, siti.Tags, "tidied: spaces trimmed, duplicates ignoring case removed")
	assert.Equal(t, []string{"IFRS", "Excel"}, siti.Skills)
	wei := a.createCandidate(token, map[string]any{"firstName": "Wei", "industry": "Banking", "skills": []string{"excel", "SQL"}})

	res := a.request(http.MethodPatch, "/v1/candidates/"+wei.Id, token, map[string]any{"version": wei.Version, "tags": []string{"hot"}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var updated api.Candidate
	res.JSON(&updated)
	assert.Equal(t, []string{"hot"}, updated.Tags)
	assert.Equal(t, []string{"excel", "SQL"}, updated.Skills, "skills not sent stay")

	var page api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates?tag=HOT", token, nil).JSON(&page)
	assert.Len(t, page.Items, 2, "tag filter ignores case")
	a.request(http.MethodGet, "/v1/candidates?skill=ifrs", token, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Siti", page.Items[0].FullName)
	assert.Equal(t, []string{"Relocate", "Hot"}, page.Items[0].Tags)

	var used api.UsedValues
	res = a.request(http.MethodGet, "/v1/candidates/used-values", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&used)
	assert.Equal(t, []string{"Banking"}, used.Industries)
	assert.Contains(t, used.Skills, "IFRS")
	assert.Equal(t, "Hot", used.Tags[0], "most used first (Hot and hot are counted apart, as typed)")

	res = a.request(http.MethodPost, "/v1/candidates", token, map[string]any{"firstName": "X", "tags": []string{string(make([]byte, 61))}})
	assert.Equal(t, http.StatusBadRequest, res.Status, "tags are at most 60 characters")
}
