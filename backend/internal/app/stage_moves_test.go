package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) stageMoves(token, query string) []api.StageMove {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/pipeline/moves"+query, token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var list api.StageMoveList
	res.JSON(&list)
	return list.Items
}

func TestTheDashboardShowsStageMovesAsATimeline(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer)
	owner, recruiter, sourcer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	company := a.createCompany(recruiter, map[string]any{"name": "Maybank"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Auditor"})
	siti := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah"})
	bala := a.createCandidate(sourcer, map[string]any{"firstName": "Bala"})

	app := a.addToJob(recruiter, job.Id, siti.Id)
	res := a.move(recruiter, app, map[string]any{"stage": "screening"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	a.addToJob(sourcer, job.Id, bala.Id)

	moves := a.stageMoves(recruiter, "")
	require.Len(t, moves, 3)
	newest := moves[0]
	assert.Equal(t, "Bala", newest.CandidateName, "newest first")
	assert.True(t, newest.FromStage.IsNull(), "added to the job")
	moved := moves[1]
	assert.Equal(t, api.StageApplied, moved.FromStage.MustGet())
	assert.Equal(t, api.StageScreening, moved.ToStage)
	assert.Equal(t, "Siti Aisyah", moved.CandidateName)
	assert.Equal(t, "Auditor", moved.JobTitle)
	assert.Equal(t, "Maybank", moved.CompanyName)
	assert.Equal(t, a.me(recruiter).User.Name, moved.ChangedByName.MustGet(), "who moved them")
	assert.Equal(t, app.Id, moved.ApplicationId)

	t.Run("one candidate's moves", func(t *testing.T) {
		mine := a.stageMoves(recruiter, "?candidateId="+siti.Id)
		require.Len(t, mine, 2)
		assert.Equal(t, api.StageScreening, mine[0].ToStage)
		assert.Equal(t, api.StageApplied, mine[1].ToStage)
		assert.Len(t, a.stageMoves(recruiter, "?limit=1"), 1)
	})

	t.Run("only moves of candidates the role may see", func(t *testing.T) {
		a.setRole(owner, "recruiter", map[string]any{
			"version": 0, "permissions": authz.PermissionsForRole(api.RoleRecruiter), "candidateScope": "own", "jobCandidates": false,
		}, http.StatusOK)
		for _, m := range a.stageMoves(recruiter, "") {
			assert.Equal(t, siti.Id, m.CandidateId, "the sourcer's candidate is hidden")
		}
	})
}
