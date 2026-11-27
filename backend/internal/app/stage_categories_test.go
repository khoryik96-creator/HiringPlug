package app_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func (a *testApp) categories(token string) api.StageCategories {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/stage-categories", token, nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.StageCategories
	res.JSON(&out)
	return out
}

// categoryItems is what is saved for cats: their keys, labels and kinds.
func categoryItems(cats []api.StageCategory) []map[string]any {
	out := make([]map[string]any, len(cats))
	for i, c := range cats {
		out[i] = map[string]any{"key": c.Key, "label": c.Label, "kind": c.Kind}
	}
	return out
}

// Stage categories: what each stage falls under (1st, 2nd and Final
// Interview), which the board, a candidate's tag and the report's funnel
// show. Owners change them, and admins once the owner gives them the right.
func TestStageCategories(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token

	cats := a.categories(recruiter)
	assert.Equal(t, "0", cats.Version)
	var names []string
	for _, c := range cats.Items {
		names = append(names, c.Label)
	}
	assert.Equal(t, []string{"Applied", "Screening", "Submitted", "1st Interview", "2nd Interview", "Final Interview", "Offer", "Placed"}, names)

	company := a.createCompany(recruiter, map[string]any{"name": "Maju"})
	job := a.createJob(recruiter, map[string]any{"companyId": company.Id, "title": "Developer"})
	assert.Equal(t, names, labels(a.jobStages(recruiter, job.Id).Steps), "a job with no template: a stage per category")

	items := categoryItems(cats.Items)
	items = slices.Insert(items, 5, map[string]any{"label": "Technical Test", "kind": "interview"})
	put := func(token string, body map[string]any) *response {
		return a.request(http.MethodPut, "/v1/stage-categories", token, body)
	}
	assert.Equal(t, http.StatusForbidden, put(recruiter, map[string]any{"version": cats.Version, "items": items}).Status)
	assert.Equal(t, http.StatusForbidden, put(admin, map[string]any{"version": cats.Version, "items": items}).Status, "owners only, until given to admins")

	res := put(owner, map[string]any{"version": cats.Version, "items": items})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var saved api.StageCategories
	res.JSON(&saved)
	require.Len(t, saved.Items, 9)
	assert.Equal(t, api.StageCategory{Key: "technical_test", Label: "Technical Test", Kind: api.OpenStageInterview}, saved.Items[5])
	assert.Equal(t, http.StatusConflict, put(owner, map[string]any{"version": cats.Version, "items": items}).Status, "saved by someone else since")

	t.Run("a candidate moved to a round falls under its category, and the report counts it", func(t *testing.T) {
		app := a.addToJob(recruiter, job.Id, a.createCandidate(recruiter, map[string]any{"firstName": "Siti"}).Id)
		assert.Equal(t, "applied", app.Category.MustGet())
		res := a.move(recruiter, app, map[string]any{"stage": "interview", "step": "interview"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&app)
		res = a.move(recruiter, app, map[string]any{"stage": "interview", "step": "technical_test"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&app)
		assert.Equal(t, "technical_test", app.Category.MustGet())

		day := func(d int) string { return time.Now().UTC().AddDate(0, 0, d).Format(time.DateOnly) }
		reached := map[string]int{}
		var rows []string
		for _, f := range a.report(owner, "?from="+day(-1)+"&to="+day(1)).Funnel {
			reached[f.Category], rows = f.Reached, append(rows, f.Label)
		}
		assert.Equal(t, []string{"Applied", "Screening", "Submitted", "1st Interview", "2nd Interview", "Technical Test", "Final Interview", "Offer", "Placed"}, rows)
		assert.Equal(t, 1, reached["technical_test"])
		assert.Equal(t, 1, reached["interview_2"], "passed on the way")
		assert.Equal(t, 0, reached["interview_final"])

		without := slices.DeleteFunc(slices.Clone(items), func(m map[string]any) bool { return m["label"] == "Technical Test" })
		gone := put(owner, map[string]any{"version": saved.Version, "items": categoryItems(saved.Items)[:5]})
		require.Equal(t, http.StatusBadRequest, gone.Status, "every kind keeps a category")
		gone = put(owner, map[string]any{"version": saved.Version, "items": without})
		require.Equal(t, http.StatusConflict, gone.Status, "%s", gone.Body)
		assert.Equal(t, api.ErrorCodeCATEGORYINUSE, gone.Error().Error.Code, "Siti is in it")

		res = a.move(recruiter, app, map[string]any{"stage": "offer"})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		renamed := categoryItems(saved.Items)
		renamed = slices.DeleteFunc(renamed, func(m map[string]any) bool { return m["key"] == "technical_test" })
		renamed[3]["label"] = "First round"
		res = put(owner, map[string]any{"version": saved.Version, "items": renamed})
		require.Equal(t, http.StatusOK, res.Status, "no one in it any more: %s", res.Body)
		res.JSON(&saved)
		assert.Equal(t, "First round", saved.Items[3].Label)
		assert.Equal(t, "interview", saved.Items[3].Key, "renamed, the same category")
	})

	t.Run("a template's stage falls under a category of its kind", func(t *testing.T) {
		steps := []map[string]any{{"label": "Applied", "stage": "applied"}, {"label": "Panel", "stage": "interview", "category": "interview_final"},
			{"label": "Offer", "stage": "offer", "category": "interview_2"}}
		res := a.request(http.MethodPost, "/v1/stage-templates", admin, map[string]any{"name": "Panels", "steps": steps})
		require.Equal(t, http.StatusBadRequest, res.Status)
		assert.Equal(t, []string{"steps.2.category"}, fieldNames(t, res.Error()))
		steps[2]["category"] = "offer"
		res = a.request(http.MethodPost, "/v1/stage-templates", admin, map[string]any{"name": "Panels", "steps": steps})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var tpl api.StageTemplate
		res.JSON(&tpl)
		assert.Equal(t, []string{"applied", "interview_final", "offer"}, []string{tpl.Steps[0].Category, tpl.Steps[1].Category, tpl.Steps[2].Category})
	})

	t.Run("the owner can give admins the right to change them", func(t *testing.T) {
		perms := append(authz.PermissionsForRole(api.RoleAdmin), api.PermissionPipelineCategoriesManage)
		a.setRole(owner, "admin", map[string]any{"version": 0, "permissions": perms, "candidateScope": "all", "jobCandidates": true}, http.StatusOK)
		now := a.categories(admin)
		assert.Equal(t, http.StatusOK, put(admin, map[string]any{"version": now.Version, "items": categoryItems(now.Items)}).Status)
	})
}
