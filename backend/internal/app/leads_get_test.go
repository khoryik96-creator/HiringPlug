package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// One lead is read as it is now, even when the list's filter would hide it.
func TestGetOneJobLead(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleSourcer, api.RoleViewer)
	token := tenant.Users[api.RoleSourcer].Token
	res := a.request(http.MethodPost, "/v1/job-leads", token, map[string]any{"companyName": "Contoh Logistik", "title": "Payroll Executive"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var lead api.JobLead
	res.JSON(&lead)
	res = a.request(http.MethodPatch, "/v1/job-leads/"+lead.Id, token, map[string]any{"version": lead.Version, "status": "ignored"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Empty(t, a.jobLeads(token, "?status=new"), "the list's filter hides it")

	res = a.request(http.MethodGet, "/v1/job-leads/"+lead.Id, token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var got api.JobLead
	res.JSON(&got)
	assert.Equal(t, lead.Id, got.Id)
	assert.Equal(t, lead.Version+1, got.Version)
	assert.Equal(t, api.LeadStatus("ignored"), got.Status)
	assert.Equal(t, "Payroll Executive", got.Title)

	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/job-leads/lead_01jbz6w0m0k8q9x2r3s4t5v6w7", token, nil).Status)
	stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/job-leads/"+lead.Id, stranger, nil).Status, "another company's lead")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/job-leads/"+lead.Id, tenant.Users[api.RoleViewer].Token, nil).Status)
}
