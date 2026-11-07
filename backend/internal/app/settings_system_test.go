package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestOwnersSeeTheirBackgroundJobsAndTryAFailedOneAgain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	c := a.createCandidate(tenant.Users[api.RoleRecruiter].Token, map[string]any{"firstName": "Lim"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", tenant.Users[api.RoleRecruiter].Token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var dead string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `UPDATE job SET status = 'dead', attempts = 5, last_error = 'smtp: 550 mailbox unavailable', finished_at = now()
		WHERE tenant_id = $1 AND kind = 'document.extract_text' RETURNING id`, tenant.ID).Scan(&dead))

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/settings/system", tenant.Users[api.RoleRecruiter].Token, nil).Status)
	var st api.SystemStatus
	res = a.request(http.MethodGet, "/v1/settings/system", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	require.Len(t, st.Failed, 1)
	assert.Equal(t, api.FailedJob{Id: dead, Kind: "document.extract_text", Attempts: 5, Error: "smtp: 550 mailbox unavailable", FailedAt: st.Failed[0].FailedAt}, st.Failed[0])
	kinds := map[string]api.JobKindStatus{}
	for _, k := range st.Jobs {
		kinds[k.Kind] = k
	}
	assert.Equal(t, 1, kinds["document.extract_text"].Failed)

	other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/settings/system/jobs/"+dead+"/retry", other, nil).Status,
		"another company cannot touch it")
	res = a.request(http.MethodPost, "/v1/settings/system/jobs/"+dead+"/retry", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	assert.Empty(t, st.Failed)
	var status string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT status FROM job WHERE id = $1`, dead).Scan(&status))
	assert.Equal(t, "pending", status)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/settings/system/jobs/"+dead+"/retry", owner, nil).Status, "only failed jobs")

	// Leave the AI queue tidy for other tests.
	_, err := testutil.Current.Admin.Exec(ctx, `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
}
