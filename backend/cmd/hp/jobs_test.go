package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// heldJob puts a job held after a restore in a company's queue.
func heldJob(t *testing.T, tenantID, kind string) string {
	t.Helper()
	var id string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `INSERT INTO job (tenant_id, queue, kind, payload, status, held_at)
		VALUES ($1, 'core', $2, '{}', 'held', now()) RETURNING id`, tenantID, kind).Scan(&id))
	return id
}

func jobStatus(t *testing.T, id string) string {
	t.Helper()
	var s string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT status FROM job WHERE id = $1`, id).Scan(&s))
	return s
}

func audited(t *testing.T, tenantID, action string) int {
	t.Helper()
	var n int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2 AND actor_id = 'system'`, tenantID, action).Scan(&n))
	return n
}

// Review of 1.47.0: only each company's owners could settle the jobs a
// restore held, one company at a time, so a company nobody looked after
// kept the whole server's alert email waiting. On the server, hp jobs
// settles every company's at once, audited in each company's log;
// releasing (it may email again), or cancelling them all, asks for -yes.
func TestHpJobsSettlesEveryCompanysHeldJobs(t *testing.T) {
	ctx := context.Background()
	t.Setenv("HP_DATABASE_ADMIN_URL", testutil.Current.AdminURL)
	a, b := testutil.Current.NewTenant(t), testutil.Current.NewTenant(t)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id IN ($1, $2)`, a.ID, b.ID)
	})
	email, webhook, push := heldJob(t, a.ID, "email.send"), heldJob(t, b.ID, "webhook.deliver"), heldJob(t, b.ID, "calendar.push")

	require.NoError(t, run(ctx, "jobs", []string{"list"}))
	require.ErrorContains(t, run(ctx, "jobs", []string{"release"}), "hp jobs release -yes")
	require.ErrorContains(t, run(ctx, "jobs", []string{"cancel"}), "hp jobs cancel -yes")
	require.ErrorContains(t, run(ctx, "jobs", []string{"cancel", "job_01jbz6w0m0k8q9x2r3s4t5v6w7"}), "no held job")
	assert.Equal(t, "held", jobStatus(t, email))

	require.NoError(t, run(ctx, "jobs", []string{"cancel", push}))
	assert.Equal(t, "dead", jobStatus(t, push))
	assert.Equal(t, 1, audited(t, b.ID, "settings.job_cancelled"))

	require.NoError(t, run(ctx, "jobs", []string{"release", "-yes"}))
	assert.Equal(t, "pending", jobStatus(t, email), "every company's")
	assert.Equal(t, "pending", jobStatus(t, webhook))
	assert.Equal(t, 1, audited(t, a.ID, "settings.held_jobs_released"), "in each company's own log")
	assert.Equal(t, 1, audited(t, b.ID, "settings.held_jobs_released"))
}

// Review of 1.47.0: paid AI paused by a restore could be turned back on
// only in Settings, by the agency HP_BACKUP_TENANT names (or the only
// one): on a server with several agencies and no HP_BACKUP_TENANT,
// nobody could. hp paid-ai does it on the server, audited.
func TestHpPaidAITurnsTheServersPaidAIOnAndOff(t *testing.T) {
	ctx := context.Background()
	t.Setenv("HP_DATABASE_ADMIN_URL", testutil.Current.AdminURL)
	a, b := testutil.Current.NewTenant(t), testutil.Current.NewTenant(t)
	t.Setenv("HP_BACKUP_TENANT", "")
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `UPDATE ai_paid_control SET paused = false, reason = NULL`)
	})
	paused := func() (bool, *string, string) {
		var p bool
		var reason *string
		var by string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT paused, reason, changed_by FROM ai_paid_control`).Scan(&p, &reason, &by))
		return p, reason, by
	}
	require.NoError(t, run(ctx, "paid-ai", []string{"off", "-reason", "Moving servers."}))
	p, reason, by := paused()
	assert.True(t, p)
	assert.Equal(t, "Moving servers.", *reason)
	assert.Equal(t, "hp paid-ai", by)
	require.NoError(t, run(ctx, "paid-ai", []string{"on"}))
	p, reason, _ = paused()
	assert.False(t, p)
	assert.Nil(t, reason)
	for _, tenant := range []string{a.ID, b.ID} {
		assert.Equal(t, 1, audited(t, tenant, "settings.paid_ai_paused"), "no agency runs the server: every company's log")
		assert.Equal(t, 1, audited(t, tenant, "settings.paid_ai_resumed"))
	}
	require.Error(t, run(ctx, "paid-ai", []string{"maybe"}))
	require.Error(t, run(ctx, "paid-ai", []string{"on", "-reason", "x"}))
}

// Review of 1.47.0: a restore of the server's own backup ran the paid AI
// work waiting in it again (billed twice, perhaps), and -resume-outbound
// silently also left paid AI on after a restore from another server. Paid
// AI work is held after any restore now, unless -resume-paid-ai; and
// -resume-outbound no longer touches paid AI.
func TestHpRestoreHoldsPaidAIWorkUnlessAskedToResumeIt(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	admin := testutil.Current.Admin
	for _, j := range [][2]string{{"match.score", `{}`}, {"document.parse_cv", `{"useAi": true}`}, {"document.parse_cv", `{"useAi": false}`},
		{"document.extract_text", `{}`}} {
		_, err := admin.Exec(ctx, `INSERT INTO job (tenant_id, queue, kind, payload) VALUES ($1, 'ai', $2, $3)`, tenant.ID, j[0], j[1])
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})
	statuses := func(target string) map[string]int {
		pool, err := database.Connect(ctx, target)
		require.NoError(t, err)
		defer pool.Close()
		out := map[string]int{}
		rows, err := pool.Query(ctx, `SELECT kind || coalesce(':' || (payload->>'useAi'), '') || ':' || status FROM job WHERE tenant_id = $1`, tenant.ID)
		require.NoError(t, err)
		for rows.Next() {
			var k string
			require.NoError(t, rows.Scan(&k))
			out[k]++
		}
		require.NoError(t, rows.Err())
		return out
	}
	paidPaused := func(target string) bool {
		pool, err := database.Connect(ctx, target)
		require.NoError(t, err)
		defer pool.Close()
		var p bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT paused FROM ai_paid_control`).Scan(&p))
		return p
	}

	target, backups := restoreTarget(t)
	made, err := backup.Run(ctx, admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: backups, Keep: 2})
	require.NoError(t, err)
	require.NoError(t, run(ctx, "restore", []string{"-yes", filepath.Base(made)}))
	assert.Equal(t, map[string]int{"match.score:held": 1, "document.parse_cv:true:held": 1, "document.parse_cv:false:pending": 1,
		"document.extract_text:pending": 1}, statuses(target), "the server's own backup: its paid AI work waits too")

	require.NoError(t, run(ctx, "restore", []string{"-yes", "-resume-paid-ai", filepath.Base(made)}))
	assert.Equal(t, map[string]int{"match.score:pending": 1, "document.parse_cv:true:pending": 1, "document.parse_cv:false:pending": 1,
		"document.extract_text:pending": 1}, statuses(target))

	other, err := backup.Run(ctx, admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: backups, Keep: 3,
		Server: "7311000000000000001"})
	require.NoError(t, err)
	require.NoError(t, run(ctx, "restore", []string{"-yes", "-resume-outbound", filepath.Base(other)}))
	assert.True(t, paidPaused(target), "-resume-outbound is about emails and webhooks, not paid AI")
	assert.Equal(t, 1, statuses(target)["match.score:held"])
	require.NoError(t, run(ctx, "restore", []string{"-yes", "-resume-paid-ai", filepath.Base(other)}))
	assert.False(t, paidPaused(target))
	assert.Contains(t, pausedAINote, "HP_BACKUP_TENANT")
	assert.Contains(t, pausedAINote, "hp paid-ai on")
}
