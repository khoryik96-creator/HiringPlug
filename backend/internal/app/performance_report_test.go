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

func (a *testApp) performance(token, query string) api.Performance {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/reports/performance"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var p api.Performance
	res.JSON(&p)
	return p
}

// The performance report: CVs sent, each stage category, placements and
// fees by month (or week, quarter, year), for chosen people or the team;
// conversions from one category to the next; days to the first CV and to
// the first placement.
func TestPerformanceReport(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	company := a.createCompany(recruiter.Token, map[string]any{"name": "Contoh Berhad"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Accountant"})
	moveTo := func(token string, app api.Application, body map[string]any) api.Application {
		res := a.move(token, app, body)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&app)
		return app
	}
	siti := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti"}).Id)
	for _, body := range []map[string]any{{"stage": "submitted"}, {"stage": "interview", "step": "interview"},
		{"stage": "interview", "step": "interview_final"}, {"stage": "offer"}} {
		siti = moveTo(recruiter.Token, siti, body)
	}
	wei := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Wei"}).Id)
	wei = moveTo(recruiter.Token, wei, map[string]any{"stage": "submitted"})
	a.placement(http.MethodPost, "/v1/applications/"+wei.Id+"/placement", recruiter.Token, map[string]any{
		"startDate": time.Now().UTC().Format(time.DateOnly), "money": map[string]any{"feeAmount": "15000", "feeCurrency": "MYR"},
	}, http.StatusCreated)
	other := a.createJob(admin.Token, map[string]any{"companyId": company.Id, "title": "Auditor"})
	aina := a.addToJob(admin.Token, other.Id, a.createCandidate(admin.Token, map[string]any{"firstName": "Aina"}).Id)
	moveTo(admin.Token, aina, map[string]any{"stage": "submitted"})

	team := a.performance(owner, "")
	assert.Equal(t, api.PerformanceGrainMonth, team.Grain)
	assert.Equal(t, time.Now().Year(), team.From.Year())
	assert.Equal(t, time.January, team.From.Month(), "this year so far")
	require.Len(t, team.Rows, int(time.Now().Month()), "a row a month")
	assert.Equal(t, 3, team.Total.CvsSent)
	assert.Equal(t, 3, team.Rows[len(team.Rows)-1].CvsSent, "all this month")
	assert.Equal(t, 1, team.Total.Categories["interview"])
	assert.Equal(t, 1, team.Total.Categories["interview_final"])
	assert.Equal(t, 1, team.Total.Categories["offer"])
	assert.Equal(t, 1, team.Total.Placements)
	assert.Equal(t, []api.MoneyTotal{{Currency: "MYR", Amount: "15000.00"}}, team.Total.Fees)
	conv := map[string]api.PerformanceConversion{}
	for _, c := range team.Conversions {
		conv[c.From+">"+c.To] = c
	}
	assert.Equal(t, 3, conv["submitted>interview"].Entered)
	assert.Equal(t, 2, conv["submitted>interview"].Reached, "Siti interviewed; Wei placed, so past it")
	assert.Equal(t, 67, conv["submitted>interview"].Percent.MustGet())
	assert.Equal(t, 1, conv["interview_final>offer"].Reached)
	assert.Equal(t, 1, conv["offer>placed"].Entered)
	assert.Equal(t, 0, conv["offer>placed"].Reached)
	firstCV, err := team.DaysToFirstCv.Average.Get()
	require.NoError(t, err)
	assert.InDelta(t, 0, firstCV, 0.1)
	assert.Equal(t, 2, team.DaysToFirstCv.Jobs)
	assert.Equal(t, 1, team.DaysToFill.Jobs)

	mine := a.performance(owner, "?user="+recruiter.ID+"&grain=quarter")
	assert.Equal(t, []string{recruiter.ID}, mine.Users)
	assert.Equal(t, 2, mine.Total.CvsSent)
	assert.Equal(t, 1, mine.DaysToFirstCv.Jobs, "only their jobs")
	assert.Len(t, mine.Rows, (int(time.Now().Month())+2)/3)
	theirs := a.performance(owner, "?user="+admin.ID)
	assert.Equal(t, 1, theirs.Total.CvsSent)
	assert.Zero(t, theirs.Total.Placements)
	assert.Empty(t, theirs.Total.Fees)

	own := a.performance(recruiter.Token, "?user="+admin.ID)
	assert.Equal(t, []string{recruiter.ID}, own.Users, "a recruiter sees only their own")
	assert.False(t, own.Team)
	assert.Equal(t, 2, own.Total.CvsSent)

	res := a.request(http.MethodGet, "/v1/reports/performance?grain=day", owner, nil)
	assert.Equal(t, http.StatusBadRequest, res.Status)
	res = a.request(http.MethodGet, "/v1/reports/performance?grain=week&from=2019-01-01", owner, nil)
	assert.Equal(t, http.StatusBadRequest, res.Status, "too many weeks")
	assert.Zero(t, a.performance(testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token, "").Total.CvsSent, "another company sees nothing")
}
