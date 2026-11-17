package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestOnlyThoseGivenItChangeACandidatesStatusAContactsRoleOrACompanysCategory(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Berhad"})
	contact := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Aisha"})
	cand := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})

	cases := []struct {
		path    string
		version int
		change  map[string]any
		other   map[string]any
	}{
		{"/v1/candidates/" + cand.Id, cand.Version, map[string]any{"availability": "looking"}, map[string]any{"currentTitle": "Accountant"}},
		{"/v1/contacts/" + contact.Id, contact.Version, map[string]any{"role": "hr"}, map[string]any{"title": "HR Manager"}},
		{"/v1/companies/" + company.Id, company.Version, map[string]any{"category": "client"}, map[string]any{"city": "Kuala Lumpur"}},
	}
	for _, c := range cases {
		body := map[string]any{"version": c.version}
		for k, v := range c.change {
			body[k] = v
		}
		res := a.request(http.MethodPatch, c.path, sourcer, body)
		assert.Equal(t, http.StatusForbidden, res.Status, "%s: %s", c.path, res.Body)
		other := map[string]any{"version": c.version}
		for k, v := range c.other {
			other[k] = v
		}
		res = a.request(http.MethodPatch, c.path, sourcer, other)
		require.Equal(t, http.StatusOK, res.Status, "other fields stay theirs to change: %s", res.Body)
		var saved struct{ Version int }
		res.JSON(&saved)
		body["version"] = saved.Version
		res = a.request(http.MethodPatch, c.path, recruiter, body)
		assert.Equal(t, http.StatusOK, res.Status, "a recruiter may (the default): %s", res.Body)
	}

}
