package app_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) namedLike(token, name string) []api.CompanyNamedLike {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/companies/named-like?name="+url.QueryEscape(name), token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.CompaniesNamedLike
	res.JSON(&out)
	return out.Items
}

// A lead's company ("MAJU FOODS BERHAD") finds the company already in
// HyPlug under another legal form first, then names much like it; never
// another tenant's companies or what is in the Bin.
func TestCompaniesNamedLikeALeadsCompany(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleViewer)
	owner := tenant.Users[api.RoleOwner].Token
	similar := a.createCompany(owner, map[string]any{"name": "Maju Food Sdn Bhd", "city": "Shah Alam"})
	same := a.createCompany(owner, map[string]any{"name": "Maju Foods Bhd", "linkedinUrl": "https://www.linkedin.com/company/maju-foods-example/"})
	a.createCompany(owner, map[string]any{"name": "Maju Foods Trading"}) // too far from "maju foods"
	a.createCompany(owner, map[string]any{"name": "Contoh Logistik Sdn Bhd"})
	binned := a.createCompany(owner, map[string]any{"name": "Maju Foods (M) Sdn Bhd"})
	res := a.bin(owner, map[string]any{"kind": "company", "id": binned.Id})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.createCompany(other.Users[api.RoleOwner].Token, map[string]any{"name": "Maju Foods Berhad"})

	found := a.namedLike(tenant.Users[api.RoleViewer].Token, "MAJU FOODS BERHAD")
	require.Len(t, found, 2, "not the other tenant's, not the Bin's, not a name too far off: %+v", found)
	assert.Equal(t, same.Id, found[0].Id, "the same name first")
	assert.True(t, found[0].SameName)
	assert.Equal(t, "https://www.linkedin.com/company/maju-foods-example", found[0].LinkedinUrl.MustGet(), "as saved")
	assert.True(t, found[0].City.IsNull())
	assert.Equal(t, similar.Id, found[1].Id, "then a name much like it")
	assert.False(t, found[1].SameName)
	assert.Equal(t, "Shah Alam", found[1].City.MustGet())

	assert.Empty(t, a.namedLike(owner, "Sdn Bhd"), "only a legal form: nothing to compare")
	assert.Empty(t, a.namedLike(owner, "Syarikat Lain Enterprise"))

	for _, blank := range []string{"", "%20%20"} {
		res := a.request(http.MethodGet, "/v1/companies/named-like?name="+blank, owner, nil)
		assert.Equal(t, http.StatusBadRequest, res.Status, "blank %q: %s", blank, res.Body)
	}
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodGet, "/v1/companies/named-like", owner, nil).Status, "no name")
	// A job feed's company name may hold a line break (or a stray NUL): read as a space.
	for _, odd := range []string{"Maju\x00Foods Berhad", "MAJU\nFOODS\nBERHAD"} {
		found := a.namedLike(owner, odd)
		require.NotEmpty(t, found, "%q", odd)
		assert.Equal(t, same.Id, found[0].Id, "%q", odd)
	}
	res = a.request(http.MethodGet, "/v1/companies/named-like?name=Maju%FFFoods", owner, nil)
	require.Equal(t, http.StatusBadRequest, res.Status, "not UTF-8: %s", res.Body)
	assert.Equal(t, []string{"name"}, fieldNames(t, res.Error()))
}

func TestCompaniesNamedLikeNeedsCompaniesRead(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleViewer)
	owner, viewer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleViewer].Token
	a.createCompany(owner, map[string]any{"name": "Maju Foods Bhd"})
	require.Len(t, a.namedLike(viewer, "Maju Foods"), 1)

	a.setRole(owner, "viewer", map[string]any{"version": 0, "candidateScope": "all", "jobCandidates": true,
		"permissions": without(authz.PermissionsForRole(api.RoleViewer), api.PermissionCompaniesRead)}, http.StatusOK)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/companies/named-like?name=Maju", viewer, nil).Status)
}

// Linking a lead to its company (Find HR, step 1): only to one of this
// tenant's companies, and the change is in the audit log.
func TestLinkingALeadToACompanyFromNamedLike(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	theirs := a.createCompany(other.Users[api.RoleOwner].Token, map[string]any{"name": "Contoh Logistik Sdn Bhd"})
	ours := a.createCompany(token, map[string]any{"name": "Contoh Logistik Bhd"})

	res := a.request(http.MethodPost, "/v1/job-leads", token, map[string]any{"companyName": "CONTOH LOGISTIK SDN BHD", "title": "HR Manager", "url": "https://example.com/ad/41"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var lead api.JobLead
	res.JSON(&lead)

	res = a.request(http.MethodPatch, "/v1/job-leads/"+lead.Id, token, map[string]any{"version": lead.Version, "companyId": theirs.Id})
	assert.Equal(t, []string{"companyId"}, fieldNames(t, res.Error()), "another tenant's company is refused, not a 500")

	found := a.namedLike(token, lead.CompanyName)
	require.Len(t, found, 1)
	require.True(t, found[0].SameName)
	res = a.request(http.MethodPatch, "/v1/job-leads/"+lead.Id, token, map[string]any{"version": lead.Version, "companyId": found[0].Id})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var linked api.JobLead
	res.JSON(&linked)
	assert.Equal(t, ours.Id, linked.CompanyId.MustGet())

	var changes string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT changes::text FROM audit_log WHERE tenant_id = $1 AND entity_id = $2 AND action = 'job_lead.updated'`, tenant.ID, lead.Id).Scan(&changes))
	assert.Contains(t, changes, ours.Id)
	assert.NotContains(t, changes, theirs.Id)
}
