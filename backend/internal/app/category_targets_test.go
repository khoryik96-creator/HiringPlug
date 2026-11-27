package app_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Targets for each stage category (1st, 2nd, Final Interview, or a
// company's own): set by owners and admins, counted per person who moved the
// candidate, each candidate once per category however often moved back in.
func TestTargetsForStageCategories(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter]

	settings := func(token string, categories map[string]any) *response {
		g := goal(nil, nil, nil, nil)
		g["categories"] = categories
		return a.request(http.MethodPut, "/v1/reports/settings", token, map[string]any{
			"targets": []any{map[string]any{"userId": nil, "period": "week", "goal": g}}, "stageWeights": []any{},
		})
	}
	assert.Equal(t, http.StatusForbidden, settings(recruiter.Token, map[string]any{"interview": 3}).Status, "admins and above set targets")
	bad := settings(owner, map[string]any{"no_such_round": 3})
	assert.Equal(t, http.StatusBadRequest, bad.Status, "only the company's categories")
	res := settings(owner, map[string]any{"interview": 3, "interview_final": 1})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var saved api.ReportSettings
	res.JSON(&saved)
	require.Len(t, saved.Targets, 1)
	require.NotNil(t, saved.Targets[0].Goal.Categories)
	assert.Equal(t, map[string]int{"interview": 3, "interview_final": 1}, *saved.Targets[0].Goal.Categories)

	company := a.createCompany(recruiter.Token, map[string]any{"name": "Maju"})
	job := a.createJob(recruiter.Token, map[string]any{"companyId": company.Id, "title": "Developer"})
	siti := a.addToJob(recruiter.Token, job.Id, a.createCandidate(recruiter.Token, map[string]any{"firstName": "Siti"}).Id)
	for _, step := range []string{"interview", "interview_2", "interview", "interview_final"} {
		res := a.move(recruiter.Token, siti, map[string]any{"stage": "interview", "step": step})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&siti)
	}

	row := rowOf(t, a.activity(recruiter.Token, "?period=week"), recruiter.ID)
	assert.Equal(t, map[string]int{"applied": 1, "interview": 1, "interview_2": 1, "interview_final": 1}, row.Done.Categories,
		"moved back into the 1st round: still one candidate")
	target, err := row.Target.Get()
	require.NoError(t, err)
	require.NotNil(t, target.Categories)
	assert.Equal(t, 3, (*target.Categories)["interview"])
	assert.Equal(t, 1, rowOf(t, a.activity(owner, "?period=week"), recruiter.ID).Done.Categories["interview_final"], "the team sees it too")
}

// A category removed after a target was set for it no longer blocks saving the
// targets: it is left out of them (no box on the page could clear it).
func TestARemovedCategoryDoesNotBlockTheTargets(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	owner := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	cats := a.categories(owner)
	items := append(categoryItems(cats.Items), map[string]any{"label": "Technical Test", "kind": "interview"})
	res := a.request(http.MethodPut, "/v1/stage-categories", owner, map[string]any{"version": cats.Version, "items": items})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&cats)
	g := goal(10, nil, nil, nil)
	g["categories"] = map[string]any{"technical_test": 2, "interview": 3}
	res = a.request(http.MethodPut, "/v1/reports/settings", owner, map[string]any{
		"targets": []any{map[string]any{"userId": nil, "period": "week", "goal": g}}, "stageWeights": []any{}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	without := slices.DeleteFunc(categoryItems(cats.Items), func(m map[string]any) bool { return m["key"] == "technical_test" })
	res = a.request(http.MethodPut, "/v1/stage-categories", owner, map[string]any{"version": cats.Version, "items": without})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	res = a.request(http.MethodGet, "/v1/reports/settings", owner, nil)
	require.Equal(t, http.StatusOK, res.Status)
	var saved api.ReportSettings
	res.JSON(&saved)
	require.Len(t, saved.Targets, 1)
	assert.Equal(t, map[string]int{"interview": 3}, *saved.Targets[0].Goal.Categories, "the removed one is left out")
	res = a.request(http.MethodPut, "/v1/reports/settings", owner, saved)
	assert.Equal(t, http.StatusOK, res.Status, "saving what the page shows works: %s", res.Body)
}
