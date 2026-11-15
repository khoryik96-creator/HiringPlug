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

func TestTheJobsListShowsOwnersAndEachJobsProgress(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleAdmin)
	recruiter, admin := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleAdmin].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Contoh Capital"})
	busy := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Risk Analyst"})
	quiet := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Credit Officer"})
	theirs := a.createJob(admin, map[string]any{"companyId": company.Id, "title": "Analyst"})
	first := a.addToJob(recruiter, busy.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti"}).Id)
	a.addToJob(recruiter, busy.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Wei"}).Id)
	require.Equal(t, http.StatusOK, a.move(recruiter, first, map[string]any{"stage": "submitted"}).Status)
	_, err := testutil.Current.Admin.Exec(context.Background(),
		`UPDATE application SET stage_changed_at = now() - interval '9 days' WHERE tenant_id = $1 AND stage = 'submitted'`, tenant.ID)
	require.NoError(t, err)

	// Owners and when each job was added; "mine" keeps the caller's own.
	var page api.JobOrderPage
	a.request(http.MethodGet, "/v1/jobs?mine=true", recruiter, nil).JSON(&page)
	titles := []string{}
	for _, j := range page.Items {
		titles = append(titles, j.Title)
		assert.Equal(t, "Test recruiter", j.OwnerName.MustGet())
		assert.WithinDuration(t, time.Now(), j.CreatedAt, time.Minute)
	}
	assert.ElementsMatch(t, []string{"Risk Analyst", "Credit Officer"}, titles)

	res := a.request(http.MethodGet, "/v1/pipeline/jobs?jobId="+busy.Id+"&jobId="+quiet.Id+"&jobId="+theirs.Id, recruiter, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var progress struct{ Items []api.JobProgress }
	res.JSON(&progress)
	byJob := map[string]api.JobProgress{}
	for _, p := range progress.Items {
		byJob[p.JobId] = p
	}
	b := byJob[busy.Id]
	assert.Equal(t, api.StageCounts{Applied: 1, Submitted: 1}, b.StageCounts)
	assert.Equal(t, 1, b.Waiting, "one candidate over 7 days in Submitted")
	assert.Equal(t, api.StageCounts{Submitted: 1}, b.WaitingCounts)
	assert.False(t, b.LastMovedAt.IsNull())
	q := byJob[quiet.Id]
	assert.Equal(t, api.StageCounts{}, q.StageCounts)
	assert.True(t, q.LastMovedAt.IsNull(), "no candidates yet")
	assert.Zero(t, q.Waiting)

	// The dashboard's week.
	var sum api.PipelineSummary
	a.request(http.MethodGet, "/v1/pipeline/summary", recruiter, nil).JSON(&sum)
	assert.Equal(t, 3, sum.OpenedLast7Days, "three open jobs added this week")
	assert.Equal(t, 2, sum.AddedLast7Days)
	assert.Equal(t, 1, sum.MovedLast7Days, "one candidate changed stage")
	assert.Zero(t, sum.MovedPrev7Days)
}
