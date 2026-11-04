package app_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) setRole(token, role string, body map[string]any, want int) api.RoleAccess {
	a.t.Helper()
	res := a.request(http.MethodPut, "/v1/access/roles/"+role, token, body)
	require.Equal(a.t, want, res.Status, "%s", res.Body)
	var out api.RoleAccess
	if want == http.StatusOK {
		res.JSON(&out)
	}
	return out
}

func (a *testApp) me(token string) api.Me {
	a.t.Helper()
	var me api.Me
	res := a.request(http.MethodGet, "/v1/me", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&me)
	return me
}

func without(perms []api.Permission, drop ...api.Permission) []api.Permission {
	return slices.DeleteFunc(slices.Clone(perms), func(p api.Permission) bool { return slices.Contains(drop, p) })
}

func TestACompanyChoosesWhatEachRoleMayDo(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token

	var settings api.AccessSettings
	res := a.request(http.MethodGet, "/v1/access", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&settings)
	require.Len(t, settings.Roles, 6)
	assert.Equal(t, api.RoleOwner, settings.Roles[0].Role)
	assert.False(t, settings.Roles[0].Editable, "the owner always has everything")
	rec := settings.Roles[2]
	assert.Equal(t, api.RoleRecruiter, rec.Role)
	assert.Equal(t, authz.PermissionsForRole(api.RoleRecruiter), rec.Permissions, "the defaults")
	assert.Equal(t, api.CandidateScopeAll, rec.CandidateScope)
	assert.Zero(t, rec.Version)
	assert.NotEmpty(t, settings.Permissions[0].Description)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/access", tenant.Users[api.RoleAdmin].Token, nil).Status, "owners only")

	c := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", owner, map[string]any{
		"expectedVersion": 0, "currentSalary": map[string]any{"amount": "8500", "currency": "MYR", "period": "month"}, "expectedSalary": nil,
	}).Status)
	noSalary := without(rec.Permissions, api.PermissionCandidatesCompensationRead, api.PermissionCandidatesCompensationUpdate)
	saved := a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": noSalary, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)
	assert.True(t, saved.Custom)
	assert.Equal(t, 1, saved.Version)

	assert.NotContains(t, a.me(recruiter).Permissions, api.PermissionCandidatesCompensationRead, "on their next request")
	var got api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+c.Id, recruiter, nil).JSON(&got)
	assert.Contains(t, got.RestrictedFields, api.RestrictedFieldCompensation)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPut, "/v1/candidates/"+c.Id+"/compensation", recruiter, map[string]any{
		"expectedVersion": 1, "currentSalary": nil, "expectedSalary": nil,
	}).Status)

	res = a.request(http.MethodPost, "/v1/api-keys", owner, map[string]any{"name": "Sync", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var key api.ApiKeyCreated
	res.JSON(&key)
	var viaKey api.Candidate
	a.request(http.MethodGet, "/v1/candidates/"+c.Id, key.Key, nil).JSON(&viaKey)
	assert.Contains(t, viaKey.RestrictedFields, api.RestrictedFieldCompensation, "API keys acting as a role follow the company's settings too")

	a.setRole(owner, "recruiter", map[string]any{"version": 0, "permissions": noSalary, "candidateScope": "all", "jobCandidates": true}, http.StatusConflict)
	a.setRole(owner, "owner", map[string]any{"version": 0, "permissions": []string{}, "candidateScope": "own", "jobCandidates": false}, http.StatusUnprocessableEntity)
	res = a.request(http.MethodPut, "/v1/access/roles/admin", owner, map[string]any{"version": 0, "permissions": []string{"identity.roles:manage"}, "candidateScope": "all", "jobCandidates": true})
	assert.Equal(t, []string{"permissions"}, fieldNames(t, res.Error()), "managing roles stays with owners")

	res = a.request(http.MethodDelete, "/v1/access/roles/recruiter", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Contains(t, a.me(recruiter).Permissions, api.PermissionCandidatesCompensationRead, "back to the defaults")
}

func TestOwnScopeHidesOtherPeoplesCandidatesEverywhere(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleSourcer)
	owner, recruiter, sourcer := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token
	own := func(role string, version int, scope string, jobs bool) {
		a.setRole(owner, role, map[string]any{"version": version, "permissions": authz.PermissionsForRole(api.Role(role)), "candidateScope": scope, "jobCandidates": jobs}, http.StatusOK)
	}
	mine := a.createCandidate(recruiter, map[string]any{"firstName": "Mine"})
	theirs := a.createCandidate(sourcer, map[string]any{"firstName": "Theirs"})
	require.Equal(t, http.StatusCreated, a.request(http.MethodPost, "/v1/notes", sourcer, map[string]any{
		"subjectType": "candidate", "subjectId": theirs.Id, "kind": "screening", "body": "Private to the sourcer's desk.",
	}).Status)
	company := a.createCompany(recruiter, map[string]any{"name": "Maju Bhd"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Accountant"})
	onJob := a.addToJob(sourcer, job.Id, theirs.Id)
	own("recruiter", 0, "own", false)

	var page api.CandidatePage
	a.request(http.MethodGet, "/v1/candidates", recruiter, nil).JSON(&page)
	assert.Equal(t, []string{mine.Id}, ids(page), "only their own")
	assert.Equal(t, mine.OwnerId.MustGet(), tenant.Users[api.RoleRecruiter].ID, "whoever adds a candidate owns them")
	for _, path := range []string{
		"/v1/candidates/" + theirs.Id, "/v1/candidates/" + theirs.Id + "/documents", "/v1/applications/" + onJob.Id + "/interviews",
	} {
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, path, recruiter, nil).Status, path)
	}
	res := a.request(http.MethodGet, "/v1/notes?subjectType=candidate&subjectId="+theirs.Id, recruiter, nil)
	assert.Equal(t, []string{"subjectId"}, fieldNames(t, res.Error()), "as for a candidate that does not exist")
	var board api.ApplicationList
	a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter, nil).JSON(&board)
	assert.Empty(t, board.Items, "not even on their own job, unless the company says so")
	var activity api.ActivityList
	a.request(http.MethodGet, "/v1/activity?entityType=candidate&entityId="+theirs.Id, recruiter, nil).JSON(&activity)
	assert.Empty(t, activity.Items)

	a.request(http.MethodGet, "/v1/candidates", owner, nil).JSON(&page)
	assert.Len(t, page.Items, 2, "owners see everyone")
	a.request(http.MethodGet, "/v1/candidates", sourcer, nil).JSON(&page)
	assert.Len(t, page.Items, 2, "other roles keep their own settings")

	t.Run("the database enforces it, whatever the query", func(t *testing.T) {
		p := authz.NewPrincipalWithAccess(tenant.ID, tenant.Users[api.RoleRecruiter].ID, "", "", api.RoleRecruiter,
			authz.PermissionsForRole(api.RoleRecruiter), authz.Scope{Users: []string{tenant.Users[api.RoleRecruiter].ID}})
		var cands, notes, apps int
		require.NoError(t, database.InTx(ctx, testutil.Current.App, p, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM candidate), (SELECT count(*) FROM note), (SELECT count(*) FROM application)`).Scan(&cands, &notes, &apps)
		}))
		assert.Equal(t, []int{1, 0, 0}, []int{cands, notes, apps})
	})

	t.Run("and candidates on their own jobs, when chosen", func(t *testing.T) {
		own("recruiter", 1, "own", true)
		assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+theirs.Id, recruiter, nil).Status)
		a.request(http.MethodGet, "/v1/jobs/"+job.Id+"/applications", recruiter, nil).JSON(&board)
		assert.Len(t, board.Items, 1)
	})

	t.Run("their team's candidates", func(t *testing.T) {
		own("recruiter", 2, "team", false)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/"+theirs.Id, recruiter, nil).Status, "no team yet: just their own")
		res := a.request(http.MethodPost, "/v1/teams", owner, map[string]any{"name": "  Finance   desk "})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var team api.TeamGroup
		res.JSON(&team)
		assert.Equal(t, "Finance desk", team.Name)
		assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPost, "/v1/teams", owner, map[string]any{"name": "finance DESK"}).Status, "names are unique")
		for _, role := range []api.Role{api.RoleRecruiter, api.RoleSourcer} {
			res := a.request(http.MethodPatch, "/v1/team/"+tenant.Users[role].ID, owner, map[string]any{"teamId": team.Id})
			require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		}
		assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/candidates/"+theirs.Id, recruiter, nil).Status, "a teammate's candidate")
		var teams api.TeamGroupList
		a.request(http.MethodGet, "/v1/teams", recruiter, nil).JSON(&teams)
		require.Len(t, teams.Items, 1)
		assert.Equal(t, 2, teams.Items[0].MemberCount)

		require.Equal(t, http.StatusNoContent, a.request(http.MethodDelete, "/v1/teams/"+team.Id, owner, nil).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/"+theirs.Id, recruiter, nil).Status, "the team is gone; its people stay")
		var members api.TeamList
		a.request(http.MethodGet, "/v1/team", owner, nil).JSON(&members)
		for _, m := range members.Items {
			assert.True(t, m.TeamId.IsNull())
		}
	})

	t.Run("handing a candidate over", func(t *testing.T) {
		res := a.request(http.MethodPut, "/v1/candidates/"+mine.Id+"/owner", recruiter, map[string]any{"ownerId": "usr_01jbz6w0m0k8q9x2r3s4t5v6w7"})
		assert.Equal(t, []string{"ownerId"}, fieldNames(t, res.Error()), "an active user of the company")
		require.Equal(t, http.StatusNoContent, a.request(http.MethodPut, "/v1/candidates/"+mine.Id+"/owner", recruiter,
			map[string]any{"ownerId": tenant.Users[api.RoleSourcer].ID}).Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/candidates/"+mine.Id, recruiter, nil).Status, "now the sourcer's")
		var got api.Candidate
		a.request(http.MethodGet, "/v1/candidates/"+mine.Id, owner, nil).JSON(&got)
		assert.Equal(t, tenant.Users[api.RoleSourcer].ID, got.OwnerId.MustGet())
		assert.NotEmpty(t, got.OwnerName.MustGet())

		require.Equal(t, http.StatusNoContent, a.request(http.MethodPut, "/v1/jobs/"+job.Id+"/owner", owner,
			map[string]any{"ownerId": tenant.Users[api.RoleSourcer].ID}).Status)
		var j api.JobOrder
		a.request(http.MethodGet, "/v1/jobs/"+job.Id, owner, nil).JSON(&j)
		assert.Equal(t, tenant.Users[api.RoleSourcer].ID, j.OwnerId.MustGet())
	})

	t.Run("my candidates, as a filter", func(t *testing.T) {
		a.request(http.MethodGet, "/v1/candidates?owner=me", sourcer, nil).JSON(&page)
		assert.Len(t, page.Items, 2, "theirs, and the one handed over")
	})
}

func ids(page api.CandidatePage) []string {
	out := make([]string, len(page.Items))
	for i, c := range page.Items {
		out[i] = c.Id
	}
	return out
}

func TestWhoMayInterviewFollowsTheCompanysSettings(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleHiringManager)
	owner := tenant.Users[api.RoleOwner].Token
	canInterview := func() bool {
		var users api.UserList
		a.request(http.MethodGet, "/v1/users", owner, nil).JSON(&users)
		for _, u := range users.Items {
			if u.Id == tenant.Users[api.RoleHiringManager].ID {
				return u.CanInterview
			}
		}
		t.Fatal("hiring manager not listed")
		return false
	}
	require.True(t, canInterview(), "by default")
	a.setRole(owner, "hiring_manager", map[string]any{
		"version": 0, "permissions": without(authz.PermissionsForRole(api.RoleHiringManager), api.PermissionInterviewsFeedback),
		"candidateScope": "all", "jobCandidates": true,
	}, http.StatusOK)
	assert.False(t, canInterview(), "the company took interview feedback away")
}
