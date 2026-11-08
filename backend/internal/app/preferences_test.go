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
