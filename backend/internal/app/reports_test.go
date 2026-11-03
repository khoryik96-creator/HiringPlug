package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) report(token, query string) api.Report {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/reports/summary"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var r api.Report
	res.JSON(&r)
	return r
}

func TestReports(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager)
	token := tenant.Users[api.RoleRecruiter].Token
	company := a.createCompany(token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	add := func(name, source string) api.Application {
		return a.addToJob(token, job.Id, a.createCandidate(token, map[string]any{"firstName": name, "source": source}).Id)
	}
	moveTo := func(app api.Application, body map[string]any) api.Application {
		res := a.move(token, app, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var moved api.Application
		res.JSON(&moved)
		return moved
	}

	// Siti: interviewed, then rejected. Wei: placed. Aina: applied only.
	siti := add("Siti", "jobstreet")
	siti = moveTo(siti, map[string]any{"stage": "interview"})
	moveTo(siti, map[string]any{"stage": "rejected", "reason": "failed_interview"})
	wei := add("Wei", "linkedin")
	a.placement(http.MethodPost, "/v1/applications/"+wei.Id+"/placement", token, map[string]any{
		"startDate": time.Now().UTC().Format(time.DateOnly), "money": map[string]any{"feeAmount": "15000", "feeCurrency": "MYR"},
	}, http.StatusCreated)
	add("Aina", "jobstreet")

	day := func(d int) string { return time.Now().UTC().AddDate(0, 0, d).Format(time.DateOnly) }
	r := a.report(token, "?from="+day(-30)+"&to="+day(1))
	reached := map[api.Stage]int{}
	for _, f := range r.Funnel {
		reached[f.Stage] = f.Reached
	}
	assert.Equal(t, map[api.Stage]int{"applied": 3, "screening": 2, "submitted": 2, "interview": 2, "offer": 1, "placed": 1}, reached,
		"the furthest stage reached: Siti was interviewed before being rejected")
	assert.Equal(t, []api.SourceRow{
		{Source: "jobstreet", Applications: 2, Interviewed: 1, Placed: 0},
		{Source: "linkedin", Applications: 1, Interviewed: 1, Placed: 1},
	}, r.Sources)
	require.Len(t, r.Recruiters, 1)
	name, _ := r.Recruiters[0].RecruiterName.Get()
	assert.Equal(t, "Test recruiter", name)
	assert.Equal(t, 1, r.Recruiters[0].Placements)
	assert.Equal(t, []api.MoneyTotal{{Currency: "MYR", Amount: "15000.00"}}, r.Recruiters[0].Fees)
	assert.False(t, r.FeesHidden)
	require.Len(t, r.TimeToFill.Jobs, 1)
	assert.Equal(t, "Accountant", r.TimeToFill.Jobs[0].JobTitle)
	days, _ := r.TimeToFill.MedianDaysToFill.Get()
	assert.Equal(t, 0, days, "opened and filled today")

	assert.Empty(t, a.report(token, "?from=2020-01-01&to=2020-12-31").Sources, "another period")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/reports/summary", tenant.Users[api.RoleHiringManager].Token, nil).Status)
	res := a.request(http.MethodGet, "/v1/reports/summary?from="+day(1)+"&to="+day(-1), token, nil)
	assert.Equal(t, []string{"from"}, fieldNames(t, res.Error()))
	assert.Empty(t, a.report(testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token, "").Sources, "another company sees nothing")
}
