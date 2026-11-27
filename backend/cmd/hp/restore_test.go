package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

// restoreTarget is an empty database to restore into, and the settings
// `hp restore` reads, pointing at it.
func restoreTarget(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	name := testutil.Current.Name + "_hp_restore"
	_, err := testutil.Current.Admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	u, err := url.Parse(testutil.Current.AdminURL)
	require.NoError(t, err)
	u.Path = "/" + name
	files, backups := t.TempDir(), t.TempDir()
	t.Setenv("HP_DATABASE_ADMIN_URL", u.String())
	t.Setenv("HP_FILES_DIR", files)
	t.Setenv("HP_BACKUP_DIR", backups)
	t.Setenv("HP_FILES_OWNER", "")
	t.Setenv("HP_BACKUP_OWNER", "")
	return u.String(), backups
}

// IMP-002, through the command people run: a backup with jobs waiting,
// running, done and failed is put back; the ones that would email, call
// webhooks or update calendars are held; local work runs again; the
// restore is recorded, from which server to which.
func TestHpRestoreHoldsWhatActsOutsideHyPlug(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	admin := testutil.Current.Admin
	for _, j := range [][2]string{{"email.send", "pending"}, {"webhook.deliver", "running"}, {"document.extract_text", "running"},
		{"email.send", "done"}, {"calendar.push", "dead"}} {
		queue := "core"
		if strings.HasPrefix(j[0], "document.") {
			queue = "ai"
		}
		_, err := admin.Exec(ctx, `INSERT INTO job (tenant_id, queue, kind, payload, status, attempts, locked_by, locked_until, finished_at)
			VALUES ($1, $2, $3, '{}', $4, 1, CASE WHEN $4 = 'running' THEN 'old' END, CASE WHEN $4 = 'running' THEN now() - interval '1 hour' END,
			CASE WHEN $4 IN ('done', 'dead') THEN now() END)`, tenant.ID, queue, j[0], j[1])
		require.NoError(t, err)
	}
	target, backups := restoreTarget(t)
	made, err := backup.Run(ctx, admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: backups, Keep: 2})
	require.NoError(t, err)
	_, err = admin.Exec(ctx, `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	require.NoError(t, err)

	// The order the guides gave, which failed before (BUG-001).
	require.NoError(t, run(ctx, "restore", []string{filepath.Base(made), "-yes"}))

	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	got := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT kind || ':' || status, attempts FROM job WHERE tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
	for rows.Next() {
		var k string
		var attempts int
		require.NoError(t, rows.Scan(&k, &attempts))
		got[k] = k
	}
	require.NoError(t, rows.Err())
	for _, want := range []string{"email.send:held", "webhook.deliver:held", "document.extract_text:pending", "email.send:done", "calendar.push:dead"} {
		assert.Contains(t, got, want)
	}
	var held int
	var source, dest *string
	var released *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT held, source_server, destination_server, released_at::text FROM restore_marker`).Scan(&held, &source, &dest, &released))
	assert.Equal(t, 2, held)
	require.NotNil(t, source)
	assert.Equal(t, *source, *dest, "the same database server here")
	assert.Nil(t, released, "until someone releases or cancels them")
}

// -resume-outbound lets them run, on purpose.
func TestHpRestoreCanResumeOutboundJobsOnPurpose(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	_, err := testutil.Current.Admin.Exec(ctx, `INSERT INTO job (tenant_id, queue, kind, payload) VALUES ($1, 'core', 'email.send', '{}')`, tenant.ID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})
	target, backups := restoreTarget(t)
	made, err := backup.Run(ctx, testutil.Current.Admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: backups, Keep: 2})
	require.NoError(t, err)
	require.NoError(t, run(ctx, "restore", []string{"-yes", "-resume-outbound", filepath.Base(made)}))
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM job WHERE tenant_id = $1`, tenant.ID).Scan(&status))
	assert.Equal(t, "pending", status)
}

// IMP-001/002: a backup made on another server (a staging copy of
// production) is put back with paid AI paused, until the operator says so.
func TestHpRestoreFromAnotherServerPausesPaidAI(t *testing.T) {
	ctx := context.Background()
	target, backups := restoreTarget(t)
	made, err := backup.Run(ctx, testutil.Current.Admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: backups, Keep: 2,
		Server: "7311000000000000001"})
	require.NoError(t, err)
	require.NoError(t, run(ctx, "restore", []string{"-yes", filepath.Base(made)}))
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var paused bool
	var by string
	require.NoError(t, pool.QueryRow(ctx, `SELECT paused, changed_by FROM ai_paid_control`).Scan(&paused, &by))
	assert.True(t, paused)
	assert.Equal(t, "hp restore", by)
	var from bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT source_server <> destination_server FROM restore_marker`).Scan(&from))
	assert.True(t, from)
}
