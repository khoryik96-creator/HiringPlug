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

// Review of 1.47.0: a restore stayed unsettled while any company had held
// jobs nobody released (the alert email waited with it), as each company
// could settle only its own; and every company saw the backup's name and
// the whole server's count, and the name of whoever paused paid AI. The
// agency that runs the server now sees and settles every company's held
// jobs; the others see only that some of theirs are held, and how many.
// Not parallel: the restore is the whole server's.
func TestTheAgencyRunningTheServerSettlesEveryCompanysHeldJobs(t *testing.T) {
	ctx := context.Background()
	op := testutil.Current.NewTenant(t, api.RoleOwner)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	idle := testutil.Current.NewTenant(t, api.RoleOwner)
	a := newTestApp(t, map[string]string{"HP_BACKUP_TENANT": op.ID})
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id IN ($1, $2)`, op.ID, other.ID)
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM restore_marker`)
		_, _ = testutil.Current.Admin.Exec(context.Background(), `UPDATE ai_paid_control SET paused = false, reason = NULL`)
	})
	held := map[string]string{}
	for tenant, kind := range map[string]string{op.ID: "webhook.deliver", other.ID: "email.send"} {
		var id string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `INSERT INTO job (tenant_id, queue, kind, payload, status, held_at)
			VALUES ($1, 'core', $2, '{}', 'held', now()) RETURNING id`, tenant, kind).Scan(&id))
		held[tenant] = id
	}
	_, err := testutil.Current.Admin.Exec(ctx, `INSERT INTO restore_marker (backup_name, source_server, destination_server, held)
		VALUES ('hiringplug-20261125-023050', '1', '2', 2)`)
	require.NoError(t, err)
	status := func(token string) api.SystemStatus {
		var st api.SystemStatus
		res := a.request(http.MethodGet, "/v1/settings/system", token, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&st)
		return st
	}
	opOwner, otherOwner := op.Users[api.RoleOwner].Token, other.Users[api.RoleOwner].Token

	st := status(opOwner)
	assert.True(t, st.HeldEveryCompany)
	require.Len(t, st.Held, 2, "every company's")
	assert.Contains(t, []string{st.Held[0].Company.MustGet(), st.Held[1].Company.MustGet()}, "Test tenant "+t.Name())
	r := st.Restore.MustGet()
	assert.True(t, r.EveryCompany)
	assert.Equal(t, "hiringplug-20261125-023050", r.BackupName.MustGet())
	assert.Equal(t, 2, r.Held)

	st = status(otherOwner)
	assert.False(t, st.HeldEveryCompany)
	require.Len(t, st.Held, 1)
	assert.True(t, st.Held[0].Company.IsNull())
	r = st.Restore.MustGet()
	assert.False(t, r.EveryCompany)
	assert.True(t, r.BackupName.IsNull(), "never the backup's name")
	assert.True(t, r.FromAnotherServer.IsNull())
	assert.Equal(t, 1, r.Held, "only their own count")
	assert.True(t, status(idle.Users[api.RoleOwner].Token).Restore.IsNull(), "nothing of theirs held: nothing to say")

	res := a.request(http.MethodPut, "/v1/settings/system/paid-ai", opOwner, map[string]any{"paused": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.Equal(t, "Paused in Settings > System.", status(otherOwner).PaidAi.Reason.MustGet(), "never who paused it")

	res = a.request(http.MethodPost, "/v1/settings/system/jobs/"+held[other.ID]+"/release", opOwner, nil)
	require.Equal(t, http.StatusOK, res.Status, "another company's job: %s", res.Body)
	res = a.request(http.MethodPost, "/v1/settings/system/held", opOwner, map[string]any{"action": "cancel"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&st)
	assert.Empty(t, st.Held)
	assert.False(t, st.Restore.MustGet().ReleasedAt.IsNull(), "settled: the alert email goes again")
	var statuses []string
	rows, err := testutil.Current.Admin.Query(ctx, `SELECT status FROM job WHERE id = ANY ($1) ORDER BY tenant_id = $2`, []string{held[op.ID], held[other.ID]}, other.ID)
	require.NoError(t, err)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		statuses = append(statuses, s)
	}
	assert.Equal(t, []string{"dead", "pending"}, statuses)
}
