package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) placement(method, path, token string, body any, want int) api.Placement {
	a.t.Helper()
	res := a.request(method, path, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var p api.Placement
	res.JSON(&p)
	return p
}

func (a *testApp) placements(token, query string) api.PlacementList {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/placements"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.PlacementList
	res.JSON(&list)
	return list
}

func TestPlacements(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager, api.RoleSourcer, api.RoleViewer)
	recruiter, manager := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleHiringManager]
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	app := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti", "lastName": "Aisyah"}).Id)
	path := "/v1/applications/" + app.Id + "/placement"
	body := map[string]any{
		"startDate": "2026-12-01",
		"money":     map[string]any{"salary": map[string]any{"amount": "8500.00", "currency": "MYR", "period": "month"}, "feePercent": "20"},
	}

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, path, tenant.Users[api.RoleSourcer].Token, body).Status)
	p := a.placement(http.MethodPost, path, recruiter.Token, body, http.StatusCreated)
	assert.Equal(t, "Siti Aisyah", p.CandidateName)
	assert.Equal(t, "Contoh Berhad", p.CompanyName)
	assert.Equal(t, 90, p.GuaranteeDays, "a 90-day guarantee unless said otherwise")
	assert.Equal(t, "2027-03-01", p.GuaranteeEndsOn.String())
	recruiterID, _ := p.RecruiterId.Get()
	assert.Equal(t, recruiter.ID, recruiterID, "credited to whoever records it unless said otherwise")
	money, err := p.Money.Get()
	require.NoError(t, err)
	fee, _ := money.FeeAmount.Get()
	assert.Equal(t, "20400.00", fee, "20% of twelve months")
	assert.Equal(t, http.StatusUnprocessableEntity, a.request(http.MethodPost, path, recruiter.Token, body).Status, "one placement per application")
	for _, item := range a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter.Token, nil).Map()["items"].([]any) {
		assert.Equal(t, "placed", item.(map[string]any)["stage"], "recording a placement places the candidate")
	}

	t.Run("salary and fee only for those allowed, enforced by the database too", func(t *testing.T) {
		list := a.placements(manager.Token, "")
		require.Len(t, list.Items, 1)
		assert.False(t, list.Items[0].Money.IsSpecified())
		assert.Equal(t, []api.PlacementRestrictedField{api.PlacementRestrictedFieldMoney}, list.Items[0].RestrictedFields)
		assert.Nil(t, list.FeeTotals)

		var rows int
		ctx := context.Background()
		conn, err := testutil.Current.App.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = conn.Rollback(ctx) }()
		_, err = conn.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true), set_config('app.permissions', 'placements:read', true)`, tenant.ID)
		require.NoError(t, err)
		require.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM placement_money`).Scan(&rows))
		assert.Zero(t, rows, "row-level security hides salary and fee")

		var changes string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
			`SELECT changes::text FROM audit_log WHERE entity_id = $1 AND action = 'placement.recorded'`, p.Id).Scan(&changes))
		assert.NotContains(t, changes, "8500")
		assert.NotContains(t, changes, "20400")
	})

	t.Run("invoiced and paid, with fee totals", func(t *testing.T) {
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{
			"version": p.Version, "invoiceStatus": "invoiced", "invoiceNumber": "INV-0042", "invoicedOn": time.Now().UTC().AddDate(0, 0, -2).Format(time.DateOnly),
		}, http.StatusOK)
		assert.Equal(t, http.StatusConflict, a.request(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{"version": p.Version - 1}).Status)
		totals := *a.placements(recruiter.Token, "").FeeTotals
		assert.Equal(t, []api.FeeTotal{{Currency: "MYR", Total: "20400.00", NotInvoiced: "0.00", Invoiced: "20400.00", Paid: "0.00"}}, totals)
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{"version": p.Version, "invoiceStatus": "paid"}, http.StatusOK)
		paidOn, _ := p.PaidOn.Get()
		assert.Equal(t, time.Now().UTC().Format(time.DateOnly), paidOn.String(), "paid today unless said otherwise")
		assert.Len(t, a.placements(recruiter.Token, "?invoiceStatus=paid").Items, 1)
		assert.Empty(t, a.placements(recruiter.Token, "?invoiceStatus=invoiced").Items)
	})

	t.Run("a fall-through is within the guarantee, and leaves the totals", func(t *testing.T) {
		res := a.request(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{"version": p.Version, "status": "fell_through", "leftOn": "2027-04-01"})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Contains(t, string(res.Body), "ended on 1 March 2027")
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{
			"version": p.Version, "status": "fell_through", "leftOn": "2027-01-15", "statusNote": "Went back to their old employer",
		}, http.StatusOK)
		assert.Equal(t, api.ReplacementStatusOwed, p.Replacement, "the client is owed a replacement")
		totals := *a.placements(recruiter.Token, "").FeeTotals
		assert.Empty(t, totals, "a fee that falls through is not counted")
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{"version": p.Version, "replacement": "provided"}, http.StatusOK)
		assert.Equal(t, api.ReplacementStatusProvided, p.Replacement)
		assert.Len(t, a.placements(manager.Token, "?status=fell_through&applicationId="+app.Id).Items, 1)
	})

	t.Run("fixed fees, clearing money, and closed applications", func(t *testing.T) {
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{
			"version": p.Version, "money": map[string]any{"feeAmount": "15000", "feeCurrency": "MYR"},
		}, http.StatusOK)
		m, _ := p.Money.Get()
		fee, _ := m.FeeAmount.Get()
		assert.Equal(t, "15000.00", fee)
		assert.False(t, m.Salary.IsSpecified() && !m.Salary.IsNull(), "the whole of the money is replaced")
		p = a.placement(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{"version": p.Version, "money": nil}, http.StatusOK)
		assert.True(t, p.Money.IsNull())

		res := a.request(http.MethodPatch, "/v1/placements/"+p.Id, recruiter.Token, map[string]any{
			"version": p.Version, "money": map[string]any{"feePercent": "20", "salary": map[string]any{"amount": "50", "currency": "MYR", "period": "hour"}},
		})
		assert.Equal(t, []string{"money.feePercent"}, fieldNames(t, res.Error()))

		other := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"}).Id)
		require.Equal(t, http.StatusOK, a.move(recruiter.Token, other, map[string]any{"stage": "rejected", "reason": "not_qualified"}).Status)
		res = a.request(http.MethodPost, "/v1/applications/"+other.Id+"/placement", recruiter.Token, map[string]any{"startDate": "2026-12-01"})
		assert.Equal(t, http.StatusUnprocessableEntity, res.Status)
		assert.Contains(t, string(res.Body), "PLACEMENT_NOT_ALLOWED")
	})

	t.Run("another company sees nothing", func(t *testing.T) {
		stranger := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter]
		assert.Empty(t, a.placements(stranger.Token, "").Items)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodPatch, "/v1/placements/"+p.Id, stranger.Token, map[string]any{"version": p.Version}).Status)
	})
}
