package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) createJob(token string, body map[string]any) api.JobOrder {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/jobs", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var j api.JobOrder
	res.JSON(&j)
	return j
}

func TestJobsCreateWithCompanyAndContact(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	maybank := a.createCompany(token, map[string]any{"name": "Maybank"})
	cimb := a.createCompany(token, map[string]any{"name": "CIMB"})
	hr := a.createContact(token, map[string]any{"companyId": maybank.Id, "firstName": "Aisyah", "lastName": "Rahman"})
	other := a.createContact(token, map[string]any{"companyId": cimb.Id, "firstName": "Wei"})

	j := a.createJob(token, map[string]any{
		"companyId": maybank.Id, "contactId": hr.Id, "title": "  Senior   Auditor ", "location": "Kuala Lumpur",
		"workType": "full_time", "workArrangement": "hybrid", "description": "Audit banks.",
	})
	assert.Equal(t, "Senior Auditor", j.Title)
	assert.Equal(t, api.JobStatusOpen, j.Status, "default status")
	assert.Equal(t, 1, j.Openings, "default openings")
	assert.Equal(t, "Maybank", j.CompanyName)
	name, _ := j.ContactName.Get()
	assert.Equal(t, "Aisyah Rahman", name)
	assert.True(t, j.Salary.IsNull(), "no budget yet")
	assert.Empty(t, j.RestrictedFields)

	res := a.request(http.MethodPost, "/v1/jobs", token, map[string]any{"companyId": maybank.Id, "contactId": other.Id, "title": "Analyst"})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"contactId"}, fieldNames(t, res.Error()), "the contact must work at the job's company")

	foreign := a.createCompany(testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token, map[string]any{"name": "Elsewhere"})
	res = a.request(http.MethodPost, "/v1/jobs", token, map[string]any{"companyId": foreign.Id, "title": "Analyst"})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"companyId"}, fieldNames(t, res.Error()))
}

func TestJobSalaryIsOnlyForPermittedRoles(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer, api.RoleHiringManager, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Grab"})
	j := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Data Engineer"})

	res := a.request(http.MethodPut, "/v1/jobs/"+j.Id+"/salary", token, map[string]any{
		"expectedVersion": 0, "range": map[string]any{"min": "9000", "max": "12000.50", "currency": "SGD", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var salary api.JobSalary
	res.JSON(&salary)
	assert.Equal(t, 1, salary.Version)

	for _, role := range []api.Role{api.RoleRecruiter, api.RoleHiringManager} {
		var got api.JobOrder
		a.request(http.MethodGet, "/v1/jobs/"+j.Id, tenant.Users[role].Token, nil).JSON(&got)
		s, err := got.Salary.Get()
		require.NoError(t, err, role)
		r, _ := s.Range.Get()
		assert.Equal(t, "9000.00", r.Min, role)
		assert.Empty(t, got.RestrictedFields, role)
	}
	for _, role := range []api.Role{api.RoleSourcer, api.RoleViewer} {
		res := a.request(http.MethodGet, "/v1/jobs/"+j.Id, tenant.Users[role].Token, nil)
		assert.NotContains(t, res.Map(), "salary", "%s: left out, not null", role)
		var got api.JobOrder
		res.JSON(&got)
		assert.Equal(t, []api.JobRestrictedField{api.JobRestrictedFieldSalary}, got.RestrictedFields, role)
		put := a.request(http.MethodPut, "/v1/jobs/"+j.Id+"/salary", tenant.Users[role].Token, map[string]any{"expectedVersion": 1, "range": nil})
		assert.Equal(t, http.StatusForbidden, put.Status, role)
	}

	// Backwards range, stale version, then clearing.
	res = a.request(http.MethodPut, "/v1/jobs/"+j.Id+"/salary", token, map[string]any{
		"expectedVersion": 1, "range": map[string]any{"min": "9000", "max": "8000", "currency": "SGD", "period": "month"},
	})
	require.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
	assert.Equal(t, []string{"range.max"}, fieldNames(t, res.Error()))
	res = a.request(http.MethodPut, "/v1/jobs/"+j.Id+"/salary", token, map[string]any{
		"expectedVersion": 0, "range": map[string]any{"min": "1", "currency": "SGD", "period": "month"},
	})
	assert.Equal(t, http.StatusConflict, res.Status, "a budget exists already")
	res = a.request(http.MethodPut, "/v1/jobs/"+j.Id+"/salary", token, map[string]any{"expectedVersion": 1, "range": nil})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var got api.JobOrder
	a.request(http.MethodGet, "/v1/jobs/"+j.Id, token, nil).JSON(&got)
	assert.True(t, got.Salary.IsNull(), "cleared")
}

func TestJobsUpdateListAndIsolation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	token := tenant.Users[api.RoleRecruiter].Token
	maybank := a.createCompany(token, map[string]any{"name": "Maybank"})
	grab := a.createCompany(token, map[string]any{"name": "Grab"})
	auditor := a.createJob(token, map[string]any{"companyId": maybank.Id, "title": "Auditor", "location": "Kuala Lumpur"})
	a.createJob(token, map[string]any{"companyId": grab.Id, "title": "Backend Engineer", "location": "Petaling Jaya"})

	res := a.request(http.MethodPatch, "/v1/jobs/"+auditor.Id, token, map[string]any{"version": auditor.Version, "status": "on_hold", "openings": 3})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var updated api.JobOrder
	res.JSON(&updated)
	assert.Equal(t, api.JobStatusOnHold, updated.Status)
	assert.Equal(t, 3, updated.Openings)
	res = a.request(http.MethodPatch, "/v1/jobs/"+auditor.Id, token, map[string]any{"version": auditor.Version, "title": "x"})
	assert.Equal(t, http.StatusConflict, res.Status, "stale version")

	var page api.JobOrderPage
	a.request(http.MethodGet, "/v1/jobs?status=on_hold", token, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Maybank", page.Items[0].CompanyName)
	a.request(http.MethodGet, "/v1/jobs?companyId="+grab.Id, token, nil).JSON(&page)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Backend Engineer", page.Items[0].Title)
	a.request(http.MethodGet, "/v1/jobs?q=petaling", token, nil).JSON(&page)
	assert.Len(t, page.Items, 1, "search includes the location")

	res = a.request(http.MethodPost, "/v1/jobs", tenant.Users[api.RoleSourcer].Token, map[string]any{"companyId": grab.Id, "title": "x"})
	assert.Equal(t, http.StatusForbidden, res.Status, "sourcers find candidates; they do not open jobs")

	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a.request(http.MethodGet, "/v1/jobs", other.Users[api.RoleOwner].Token, nil).JSON(&page)
	assert.Empty(t, page.Items)
	res = a.request(http.MethodGet, "/v1/jobs/"+auditor.Id, other.Users[api.RoleOwner].Token, nil)
	assert.Equal(t, http.StatusNotFound, res.Status)
}

// Even a raw query (a bug, or an endpoint someone forgot to protect) cannot
// read a job's salary budget without jobs.salary:read, or write one without
// jobs.salary:update: the database refuses.
func TestTheDatabaseProtectsJobSalaries(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	j := a.createJob(token, map[string]any{"companyId": a.createCompany(token, map[string]any{"name": "Acme"}).Id, "title": "Auditor"})
	res := a.request(http.MethodPut, "/v1/jobs/"+j.Id+"/salary", token, map[string]any{
		"expectedVersion": 0, "range": map[string]any{"min": "7000", "currency": "MYR", "period": "month"},
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	count := func(p authz.Principal) int {
		var n int
		require.NoError(t, database.InTx(ctx, testutil.Current.App, p, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM job_order_salary WHERE job_order_id = $1", j.Id).Scan(&n)
		}))
		return n
	}
	assert.Equal(t, 1, count(authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleHiringManager)))
	assert.Zero(t, count(authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleSourcer)))
	assert.Zero(t, count(authz.System(testutil.Current.NewTenant(t, api.RoleOwner).ID)))

	// A hiring manager may read the budget but not change it.
	err := database.InTx(ctx, testutil.Current.App, authz.NewPrincipal(tenant.ID, "usr_x", "", "", api.RoleHiringManager), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE job_order_salary SET min_amount = 1 WHERE job_order_id = $1", j.Id)
		if err == nil && tag.RowsAffected() != 0 {
			t.Errorf("updated %d rows without jobs.salary:update", tag.RowsAffected())
		}
		return err
	})
	require.NoError(t, err)
}
