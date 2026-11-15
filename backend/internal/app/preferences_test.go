package app_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestEachPersonChoosesTheirOwnCandidateColumns(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleSourcer)
	recruiter, sourcer := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleSourcer].Token

	get := func(token string) api.Preferences {
		var p api.Preferences
		r := a.request(http.MethodGet, "/v1/me/preferences", token, nil)
		require.Equal(t, http.StatusOK, r.Status)
		r.JSON(&p)
		return p
	}
	assert.True(t, get(recruiter).CandidateColumns.IsNull(), "the default columns until one is chosen")

	chosen := []any{"nationality", "residence", "workAuthorization", "status"}
	r := a.request(http.MethodPut, "/v1/me/preferences", recruiter, map[string]any{"candidateColumns": chosen})
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, []api.CandidateColumn{"nationality", "residence", "workAuthorization", "status"}, get(recruiter).CandidateColumns.MustGet(), "kept, in order")
	assert.True(t, get(sourcer).CandidateColumns.IsNull(), "someone else's choice is theirs alone")

	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPut, "/v1/me/preferences", recruiter,
		map[string]any{"candidateColumns": []any{"status", "status"}}).Status)
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPut, "/v1/me/preferences", recruiter,
		map[string]any{"candidateColumns": []any{"shoeSize"}}).Status)

	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/me/preferences", recruiter, map[string]any{"candidateColumns": nil}).Status)
	assert.True(t, get(recruiter).CandidateColumns.IsNull(), "back to the default")
}

func TestEachPersonArrangesTheirOwnDashboard(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	me, other := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	prefs := func(token string) api.Preferences {
		var p api.Preferences
		res := a.request(http.MethodGet, "/v1/me/preferences", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&p)
		return p
	}
	assert.True(t, prefs(me).DashboardBlocks.IsNull(), "the default order")

	// Columns first; then the dashboard's order, sent on its own, keeps them.
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/me/preferences", me, map[string]any{"candidateColumns": []string{"status", "email"}}).Status)
	order := []string{"openJobs", "needsYou", "numbers"}
	res := a.request(http.MethodPut, "/v1/me/preferences", me, map[string]any{"dashboardBlocks": order})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	got := prefs(me)
	assert.Equal(t, []api.DashboardBlock{"openJobs", "needsYou", "numbers"}, got.DashboardBlocks.MustGet())
	assert.Equal(t, []api.CandidateColumn{"status", "email"}, got.CandidateColumns.MustGet(), "left out: kept")
	assert.True(t, prefs(other).DashboardBlocks.IsNull(), "someone else's dashboard is theirs")

	assert.Equal(t, []string{"dashboardBlocks"}, fieldNames(t, a.request(http.MethodPut, "/v1/me/preferences", me,
		map[string]any{"dashboardBlocks": []string{"numbers", "numbers"}}).Error()))
	assert.Equal(t, http.StatusBadRequest, a.request(http.MethodPut, "/v1/me/preferences", me, map[string]any{"dashboardBlocks": []string{"weather"}}).Status)

	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/me/preferences", me, map[string]any{"dashboardBlocks": nil}).Status)
	assert.True(t, prefs(me).DashboardBlocks.IsNull(), "back to the default order")
	assert.Equal(t, []api.CandidateColumn{"status", "email"}, prefs(me).CandidateColumns.MustGet())
}
