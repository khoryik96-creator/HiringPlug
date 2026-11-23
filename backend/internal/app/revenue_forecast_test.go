package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) forecast(token string) api.Forecast {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/reports/forecast", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var f api.Forecast
	res.JSON(&f)
	return f
}

func TestRevenueForecast(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	token := owner.Token

	// The client's rate: 20% of the annual salary. The job's budget: RM 5,000
	// to 7,000 a month, so RM 72,000 a year and a fee of RM 14,400.
	company := a.createCompany(token, map[string]any{"name": "Maju Foods Berhad"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/companies/"+company.Id+"/terms", token, map[string]any{
		"version": 0, "tiers": []any{rate(nil, nil, "20", 30, 90, nil)}, "salaryCurrency": "MYR", "signedOn": nil, "notes": nil,
	}).Status)
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Finance Manager"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/salary", token, map[string]any{
		"expectedVersion": 0, "range": map[string]any{"min": "5000", "max": "7000", "currency": "MYR", "period": "month"},
	}).Status)
	noBudget := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Clerk"})

	at := func(jobID, name, stage string) api.Application {
		app := a.addToJob(token, jobID, a.createCandidate(token, map[string]any{"firstName": name}).Id)
		if stage != "applied" {
			res := a.move(token, app, map[string]any{"stage": stage})
			require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
			res.JSON(&app)
		}
		return app
	}
	at(job.Id, "Siti", "submitted")       // 10%: 1,440 in two months
	at(job.Id, "Wei", "interview")        // 30%: 4,320 next month
	at(job.Id, "Aina", "offer")           // 70%: 10,080 this month
	at(job.Id, "Ali", "applied")          // not counted yet
	at(noBudget.Id, "Kumar", "submitted") // no budget: listed as a gap
	start := time.Now().In(a.deps.Config.TimeZone).AddDate(0, 1, 0)
	placed := at(job.Id, "Mei", "offer")
	a.placement(http.MethodPost, "/v1/applications/"+placed.Id+"/placement", token, map[string]any{
		"startDate": start.Format(time.DateOnly), "recruiterId": recruiter.ID, "money": map[string]any{"feeAmount": "15000", "feeCurrency": "MYR"},
	}, http.StatusCreated) // 100% of its recorded fee, in its start month
	invoiced := at(job.Id, "Raj", "offer")
	p := a.placement(http.MethodPost, "/v1/applications/"+invoiced.Id+"/placement", token, map[string]any{
		"startDate": time.Now().UTC().Format(time.DateOnly), "money": map[string]any{"feeAmount": "9000", "feeCurrency": "MYR"},
	}, http.StatusCreated)
	a.placement(http.MethodPatch, "/v1/placements/"+p.Id, token, map[string]any{
		"version": p.Version, "invoiceStatus": "invoiced", "invoiceNumber": "INV-1", "invoicedOn": time.Now().In(a.deps.Config.TimeZone).Format(time.DateOnly),
	}, http.StatusOK) // invoiced: no longer expected

	f := a.forecast(token)
	month := func(ahead int) string {
		now := time.Now().In(a.deps.Config.TimeZone)
		return time.Date(now.Year(), now.Month()+time.Month(ahead), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	}
	expected := map[string]string{}
	for _, m := range f.Months {
		require.Len(t, m.Totals, 1)
		assert.Equal(t, "MYR", m.Totals[0].Currency)
		expected[m.Month] = m.Totals[0].Expected
	}
	assert.Equal(t, map[string]string{
		month(0): "10080.00",
		month(1): "19320.00", // Wei's 4,320 and Mei's placement, 15,000
		month(2): "1440.00",
	}, expected)
	require.Len(t, f.Lines, 4)
	assert.Equal(t, "Mei", f.Lines[0].CandidateName, "most expected first")
	assert.Equal(t, api.ForecastLineFeeFromPlacement, f.Lines[0].FeeFrom)
	assert.Equal(t, "Test recruiter", f.Lines[0].RecruiterName.MustGet(), "the recruiter credited with the placement")
	assert.Equal(t, api.ForecastLineFeeFromTerms, f.Lines[1].FeeFrom)
	assert.Equal(t, "14400.00", f.Lines[1].Fee)
	assert.Equal(t, owner.ID, f.Lines[1].RecruiterId.MustGet(), "else the job's owner")
	require.Len(t, f.Gaps, 1)
	assert.Equal(t, api.ForecastGap{ApplicationId: f.Gaps[0].ApplicationId, CandidateName: "Kumar", JobTitle: "Clerk", Stage: "submitted", Reason: "no_salary_budget"}, f.Gaps[0])
	require.Len(t, f.Recruiters, 2)
	assert.Equal(t, owner.ID, f.Recruiters[0].RecruiterId.MustGet(), "most expected first")
	assert.Equal(t, "15840.00", f.Recruiters[0].Totals[0].Expected)
	assert.Equal(t, "43200.00", f.Recruiters[0].Totals[0].Full, "fees before weighting")
	assert.Equal(t, "15000.00", f.Recruiters[1].Totals[0].Expected)

	t.Run("weights from Settings", func(t *testing.T) {
		require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/reports/settings", token, map[string]any{
			"targets": []any{}, "stageWeights": []map[string]any{{"stage": "offer", "percent": 50, "monthsAhead": 1}},
		}).Status)
		for _, l := range a.forecast(token).Lines {
			if l.CandidateName == "Aina" {
				assert.Equal(t, "7200.00", l.Expected)
				assert.Equal(t, month(1), l.Month)
			}
		}
	})

	t.Run("fees: owners and admins only", func(t *testing.T) {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/reports/forecast", recruiter.Token, nil).Status)
		mine := xlsxRows(t, a.request(http.MethodGet, "/v1/reports/summary/export?format=xlsx", recruiter.Token, nil))
		assert.Contains(t, mine, "Activity")
		assert.NotContains(t, mine, "Forecast by month", "no fees in a recruiter's file")
		all := xlsxRows(t, a.request(http.MethodGet, "/v1/reports/summary/export?format=xlsx", token, nil))
		for _, name := range []string{"Activity", "Forecast by month", "Forecast by recruiter", "Forecast detail"} {
			assert.Contains(t, all, name)
		}
		assert.Len(t, all["Forecast detail"], 5, "a heading and four lines")
		csv := a.request(http.MethodGet, "/v1/reports/summary/export?format=csv", token, nil)
		assert.True(t, strings.Contains(string(csv.Body), "Forecast by recruiter"))
	})

	t.Run("no fee values in the audit log", func(t *testing.T) {
		var saved, money int
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
			`SELECT count(*), count(*) FILTER (WHERE changes::text ~ '[0-9]{4}') FROM audit_log WHERE tenant_id = $1 AND action = 'reports.settings_saved'`,
			tenant.ID).Scan(&saved, &money))
		assert.Equal(t, 1, saved, "the settings change is audited")
		assert.Zero(t, money)
	})

	other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	assert.Empty(t, a.forecast(other).Lines, "another company sees nothing of this one")
}
