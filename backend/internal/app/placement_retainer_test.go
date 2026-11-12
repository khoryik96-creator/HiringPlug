package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A retained search: the client pays stages on the job while the search runs,
// and what they add up to is taken off the fee of the first placement.
func TestJobRetainer(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager)
	recruiter, manager := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleHiringManager].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Seri Harvest Berhad"})
	hiring := a.createContact(recruiter, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "lastName": "Rahman",
		"email": "aisyah." + tenant.ID + "@seriharvest.test"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "contactId": hiring.Id, "title": "Chief Financial Officer"})
	path := "/v1/jobs/" + job.Id + "/retainer"
	retainer := func(token string, body any, want int) api.JobRetainer {
		t.Helper()
		method := http.MethodGet
		if body != nil {
			method = http.MethodPut
		}
		res := a.request(method, path, token, body)
		require.Equal(t, want, res.Status, "%s", res.Body)
		var r api.JobRetainer
		res.JSON(&r)
		return r
	}

	r := retainer(recruiter, nil, http.StatusOK)
	assert.Equal(t, 0, r.Version, "none yet")
	assert.Empty(t, r.Stages)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, path, manager, nil).Status, "money: fees only")
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/jobs/jo_00000000000000000000000000/retainer", recruiter, nil).Status)

	stages := []any{
		map[string]any{"label": "Engagement", "amount": "15000", "dueKind": "after_signing", "dueDays": 0, "invoiceStatus": "paid",
			"invoiceNumber": "INV-0007", "invoicedOn": "2026-11-01", "paidOn": "2026-11-10"},
		map[string]any{"label": "Shortlist", "amount": "15000", "dueKind": "on_date", "dueOn": "2026-12-15"},
		map[string]any{"label": "Interviews done", "amount": "5000", "dueKind": "after_start", "dueDays": 14},
	}
	bad := a.request(http.MethodPut, path, recruiter, map[string]any{"version": 0, "currency": "MYR", "stages": []any{
		map[string]any{"label": " ", "amount": "15000", "dueKind": "on_date"},
	}})
	require.Equal(t, http.StatusBadRequest, bad.Status)
	assert.ElementsMatch(t, []string{"stages.0.label", "stages.0.dueOn"}, fieldNames(t, bad.Error()))

	r = retainer(recruiter, map[string]any{"version": 0, "currency": "MYR", "signedOn": "2026-11-01", "stages": stages}, http.StatusOK)
	assert.Equal(t, 1, r.Version)
	assert.Equal(t, "35000.00", r.Total)
	require.Len(t, r.Stages, 3)
	assert.Equal(t, "2026-11-01", r.Stages[0].DueOn.MustGet().String(), "0 days after signing")
	assert.True(t, r.Stages[2].DueOn.IsNull(), "after the start: not known until placed")
	assert.True(t, r.PlacementId.IsNull())
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPut, path, recruiter, map[string]any{"version": 0, "currency": "MYR", "stages": stages}).Status)

	t.Run("money stays hidden: the database too", func(t *testing.T) {
		ctx := context.Background()
		conn, err := testutil.Current.App.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = conn.Rollback(ctx) }()
		_, err = conn.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true), set_config('app.permissions', 'placements:read', true)`, tenant.ID)
		require.NoError(t, err)
		var rows int
		require.NoError(t, conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM job_retainer) + (SELECT count(*) FROM job_retainer_stage)`).Scan(&rows))
		assert.Zero(t, rows, "row-level security hides retainers")

		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
			`SELECT changes::text FROM audit_log WHERE entity_id = $1 AND action = 'job.retainer_set'`, job.Id).Scan(&changes))
		assert.NotContains(t, changes, "15000", "never the amounts")
	})

	place := func() api.Placement {
		appl := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah"}).Id)
		return a.placement(http.MethodPost, "/v1/applications/"+appl.Id+"/placement", recruiter, map[string]any{
			"startDate": "2027-01-04",
			"money":     map[string]any{"salary": map[string]any{"amount": "25000.00", "currency": "MYR", "period": "month"}, "feePercent": "15"},
		}, http.StatusCreated)
	}
	p := place()
	money := p.Money.MustGet()
	assert.Equal(t, "45000.00", money.FeeAmount.MustGet())
	off := money.Retainer.MustGet()
	assert.Equal(t, "35000.00", off.Total)
	assert.Equal(t, "10000.00", off.Balance.MustGet(), "the retainer is taken off the fee")
	assert.Equal(t, "2027-01-18", off.Stages[2].DueOn.MustGet().String(), "14 days after the start")
	assert.Equal(t, p.Id, retainer(recruiter, nil, http.StatusOK).PlacementId.MustGet())
	assert.True(t, place().Money.MustGet().Retainer.IsNull(), "only the first placement on the job")
	assert.Equal(t, []api.PlacementRestrictedField{api.PlacementRestrictedFieldMoney},
		a.placements(manager, "?applicationId="+p.ApplicationId).Items[0].RestrictedFields)

	t.Run("its invoice is for the balance", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/placements/"+p.Id+"/invoice/file?format=docx", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		doc := docxText(t, res.Body)
		assert.Contains(t, doc, "less the retainer of MYR 35,000.00")
		assert.Contains(t, doc, "MYR 10,000.00")
		assert.NotContains(t, doc, "MYR 45,000.00")
	})

	t.Run("cleared", func(t *testing.T) {
		r = retainer(recruiter, map[string]any{"version": 1, "stages": []any{}}, http.StatusOK)
		assert.Equal(t, 0, r.Version)
		assert.True(t, a.placements(recruiter, "?applicationId="+p.ApplicationId).Items[0].Money.MustGet().Retainer.IsNull())
	})
}
