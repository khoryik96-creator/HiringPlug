package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// An agency names itself, and its formatted CVs are named after it the way
// CV Studio names them: "Hyppies CV - Siti Aisyah.docx".
func TestAnAgencyRenamesItself(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/agency", recruiter, map[string]any{"name": "Hyppies"}).Status)
	res := a.request(http.MethodPut, "/v1/agency", owner, map[string]any{"name": "   "})
	require.Equal(t, http.StatusBadRequest, res.Status)
	assert.Equal(t, []string{"name"}, fieldNames(t, res.Error()))

	var before api.Me
	a.request(http.MethodGet, "/v1/me", owner, nil).JSON(&before)
	res = a.request(http.MethodPut, "/v1/agency", owner, map[string]any{"name": "  Hyppies  "})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var renamed api.Tenant
	res.JSON(&renamed)
	assert.Equal(t, "Hyppies", renamed.Name)
	assert.Equal(t, before.Tenant.CareersSlug, renamed.CareersSlug, "the careers page address stays")

	var me api.Me
	a.request(http.MethodGet, "/v1/me", recruiter, nil).JSON(&me)
	assert.Equal(t, "Hyppies", me.Tenant.Name)

	c := a.createCandidate(recruiter, map[string]any{"firstName": "Azri", "lastName": "Abdul Rashid"})
	out, _ := a.formatCv(recruiter, c.Id, map[string]any{}, http.StatusCreated)
	assert.Equal(t, "Hyppies CV - Azri Abdul Rashid.docx", out.FileName)
}
