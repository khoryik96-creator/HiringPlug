package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The dashboard's job numbers for chosen people: only the jobs they own, and
// every number (in progress, moved, placed) about those jobs; no one chosen
// is the whole company's.
func TestTheDashboardShowsChosenPeoplesJobs(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer)
	owner := tenant.Users[api.RoleOwner].Token
	aina, wei := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleSourcer]
	company := a.createCompany(owner, map[string]any{"name": "Maju"})
	ainas := a.createJob(owner, map[string]any{"companyId": company.Id, "title": "Accountant"})
	weis := a.createJob(owner, map[string]any{"companyId": company.Id, "title": "Engineer"})
	nobodys := a.createJob(owner, map[string]any{"companyId": company.Id, "title": "Nobody's"})
	for job, to := range map[string]any{ainas.Id: aina.ID, weis.Id: wei.ID, nobodys.Id: nil} {
		res := a.request(http.MethodPut, "/v1/jobs/"+job+"/owner", owner, map[string]any{"ownerId": to})
		require.Equal(t, http.StatusNoContent, res.Status, "%s", res.Body)
	}
	for range 2 {
		a.addToJob(owner, ainas.Id, a.createCandidate(owner, map[string]any{"firstName": "Siti"}).Id)
	}
	moved := a.addToJob(owner, weis.Id, a.createCandidate(owner, map[string]any{"firstName": "Farid"}).Id)
	require.Equal(t, http.StatusOK, a.move(owner, moved, map[string]any{"stage": "screening"}).Status)

	summary := func(query string) api.PipelineSummary {
		res := a.request(http.MethodGet, "/v1/pipeline/summary"+query, owner, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var s api.PipelineSummary
		res.JSON(&s)
		return s
	}
	titles := func(s api.PipelineSummary) []string {
		var out []string
		for _, j := range s.Jobs {
			out = append(out, j.Title)
		}
		return out
	}
	all := summary("")
	assert.Equal(t, 3, all.OpenJobs)
	assert.Equal(t, 3, all.AddedLast7Days)

	mine := summary("?owner=" + aina.ID)
	assert.Equal(t, []string{"Accountant"}, titles(mine))
	assert.Equal(t, 2, mine.StageCounts.Applied)
	assert.Equal(t, 2, mine.AddedLast7Days, "only their jobs' candidates")
	assert.Zero(t, mine.MovedLast7Days, "Farid moved on Wei's job")

	both := summary("?owner=" + aina.ID + "&owner=" + wei.ID)
	assert.ElementsMatch(t, []string{"Accountant", "Engineer"}, titles(both))
	assert.Equal(t, 1, both.MovedLast7Days)
	assert.Equal(t, 3, both.AddedLast7Days)
}
