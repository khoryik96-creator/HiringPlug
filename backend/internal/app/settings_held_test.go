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

// IMP-002: the jobs a restore held (emails, webhooks...) wait in Settings >
// System, for the company's owners and admins to release or cancel, each or
// all at once; audited like a job tried again; other companies' are never
// touched.
func TestOwnersReleaseOrCancelTheJobsARestoreHeld(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})
	held := make([]string, 4)
	for i, kind := range []string{"email.send", "webhook.deliver", "calendar.push", "automation.run"} {
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `INSERT INTO job (tenant_id, queue, kind, payload, status, held_at, run_after)
			VALUES ($1, 'core', $2, '{}', 'held', now(), now() - interval '1 day') RETURNING id`, tenant.ID, kind).Scan(&held[i]))
	}

	assert.Equal(t, http.StatusForbidden, a.request(http.MethodPost, "/v1/settings/system/jobs/"+held[0]+"/release", tenant.Users[api.RoleRecruiter].Token, nil).Status)
	var st api.SystemStatus
	res := a.request(http.MethodGet, "/v1/settings/system", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	require.Len(t, st.Held, 4)
	assert.Equal(t, "email.send", st.Held[0].Kind)
	other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/settings/system/jobs/"+held[0]+"/release", other, nil).Status)
	assert.Equal(t, http.StatusOK, a.request(http.MethodPost, "/v1/settings/system/held", other, map[string]any{"action": "cancel"}).Status)

	res = a.request(http.MethodPost, "/v1/settings/system/jobs/"+held[0]+"/release", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	assert.Len(t, st.Held, 3)
	res = a.request(http.MethodPost, "/v1/settings/system/jobs/"+held[1]+"/cancel", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	assert.Len(t, st.Held, 2)
	require.Len(t, st.Failed, 1, "a cancelled job shows as failed, saying why")
	assert.Equal(t, "Cancelled after the server was restored: not run.", st.Failed[0].Error)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodPost, "/v1/settings/system/jobs/"+held[1]+"/release", owner, nil).Status, "only held jobs")
	res = a.request(http.MethodPost, "/v1/settings/system/held", owner, map[string]any{"action": "release"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	assert.Empty(t, st.Held)

	statuses := map[string]string{}
	for _, id := range held {
		var s string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT status FROM job WHERE id = $1`, id).Scan(&s))
		statuses[id] = s
	}
	assert.Equal(t, map[string]string{held[0]: "pending", held[1]: "dead", held[2]: "pending", held[3]: "pending"}, statuses)
	var actions []string
	rows, err := testutil.Current.Admin.Query(ctx, `SELECT action FROM audit_log WHERE tenant_id = $1 AND action LIKE 'settings.%' ORDER BY id`, tenant.ID)
	require.NoError(t, err)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		actions = append(actions, s)
	}
	assert.Equal(t, []string{"settings.job_released", "settings.job_cancelled", "settings.held_jobs_released"}, actions)
}
