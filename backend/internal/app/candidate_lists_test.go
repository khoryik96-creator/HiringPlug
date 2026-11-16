package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The specialization, IT skills and certifications lists start with the
// agency's usual items; in Settings any can be renamed, removed or added, and
// candidates keep the words they have.
func TestSpecializationSkillAndCertificationListsAreTheAgencys(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	list := func(name api.FieldListName) api.FieldList {
		var all api.FieldSettings
		a.request(http.MethodGet, "/v1/settings/fields", recruiter, nil).JSON(&all)
		for _, l := range all.Lists {
			if l.Name == name {
				return l
			}
		}
		t.Fatalf("no %s list", name)
		return api.FieldList{}
	}
	labels := func(l api.FieldList) []string {
		var out []string
		for _, o := range l.Options {
			out = append(out, o.Label)
		}
		return out
	}
	skills := list(api.FieldListNameItSkill)
	assert.Contains(t, labels(skills), "Solidity/Blockchain Tech")
	assert.False(t, skills.Options[0].BuiltIn, "removable")
	assert.Equal(t, []string{"ACCA", "CFA", "CIA", "CIMA", "CPA", "MIA", "ICAEW", "CISCO", "CompTIA", "Palo Alto", "Fortinet"},
		labels(list(api.FieldListNameCertification)))
	require.NotEmpty(t, labels(list(api.FieldListNameSpecialization)))

	c := a.createCandidate(recruiter, map[string]any{"firstName": "Aina", "skills": []string{"PHP"}, "certifications": []string{"ACCA"}})
	res := a.request(http.MethodPut, "/v1/settings/fields/lists/it_skill", owner, map[string]any{"options": []map[string]any{
		{"key": "Golang", "label": "Go (Golang)", "hidden": false}, {"label": "Rust", "hidden": false},
	}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"Go (Golang)", "Rust"}, labels(list(api.FieldListNameItSkill)), "renamed, added, the rest removed")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/settings/fields/lists/certification", recruiter,
		map[string]any{"options": []map[string]any{{"label": "PMP", "hidden": false}}}).Status, "only those who manage settings")

	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter, nil).JSON(&got)
	assert.Equal(t, []string{"PHP"}, got.Skills, "a candidate keeps the words they have")
	assert.Equal(t, []string{"ACCA"}, got.Certifications)
}
