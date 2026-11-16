package alerts_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/alerts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

type inbox struct{ got []mail.Message }

func (i *inbox) Send(_ context.Context, m mail.Message) error { i.got = append(i.got, m); return nil }

// The checks read as the app's own restricted role: what the worker has.
func TestTheServerSaysWhenItNeedsSomeone(t *testing.T) {
	ctx := context.Background()
	admin := testutil.Current.Admin
	tenant := testutil.Current.NewTenant(t)
	clock := time.Now()
	disk := struct{ free, total uint64 }{free: 50 << 30, total: 100 << 30}
	checks := alerts.Checks{DB: testutil.Current.App, FilesDir: "/data/files", Now: func() time.Time { return clock },
		Disk: func(string) (uint64, uint64, error) { return disk.free, disk.total, nil }}
	box := &inbox{}
	// The backup service checks in all along (by the test's clock).
	seen := func() {
		_, err := admin.Exec(ctx, `UPDATE backup_control SET service_seen_at = $1`, clock)
		require.NoError(t, err)
	}
	w := &alerts.Watcher{Checks: checks, Mail: box, To: []string{"it@agency.test"}, Server: "https://app.agency.test",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	keys := func() []string {
		ps, err := checks.Problems(ctx)
		require.NoError(t, err)
		var out []string
		for _, p := range ps {
			out = append(out, p.Key)
		}
		return out
	}

	// All is well: a backup an hour ago, copied off the server; the service just checked in.
	_, err := admin.Exec(ctx, `UPDATE backup_control SET auto = true, service_seen_at = now()`)
	require.NoError(t, err)
	var run int64
	require.NoError(t, admin.QueryRow(ctx, `INSERT INTO backup_run (name, started_at, finished_at, status, size_bytes, offsite_status, offsite_at)
		VALUES ('hiringplug-20261115-020000', now() - interval '1 hour', now(), 'done', 10, 'done', now()) RETURNING id`).Scan(&run))
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DELETE FROM backup_run WHERE id = $1`, run) })
	assert.Empty(t, keys())
	require.NoError(t, w.Check(ctx))
	assert.Empty(t, box.got, "nothing to say")

	// Things go wrong: the off-site copy failed, a task failed for good, the disk fills up.
	_, err = admin.Exec(ctx, `UPDATE backup_run SET offsite_status = 'failed', offsite_error = 'rclone: access denied' WHERE id = $1`, run)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, `INSERT INTO job (tenant_id, queue, kind, payload, status, finished_at) VALUES ($1, 'core', 'email.send', '{}', 'dead', now())`, tenant.ID)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID) })
	disk.free = 3 << 30
	assert.ElementsMatch(t, []string{"offsite-failed", "jobs-failed", "disk-full"}, keys())
	require.NoError(t, w.Check(ctx))
	require.Len(t, box.got, 1)
	sent := box.got[0]
	assert.Equal(t, []string{"it@agency.test"}, sent.To)
	assert.Equal(t, "HiringPlug needs you: 3 problem(s) on the server", sent.Subject)
	assert.Contains(t, sent.Body, "rclone: access denied")
	assert.Contains(t, sent.Body, "3.0 GB free of 100.0 GB")
	assert.Contains(t, sent.Body, "https://app.agency.test")

	// Still there a little later: not emailed again; a day later, it is.
	clock = clock.Add(time.Hour)
	seen()
	require.NoError(t, w.Check(ctx))
	assert.Len(t, box.got, 1, "once a day while it lasts")
	clock = clock.Add(24 * time.Hour)
	seen()
	require.NoError(t, w.Check(ctx))
	assert.Len(t, box.got, 2)

	// Fixed: one "over now" email, then quiet.
	_, err = admin.Exec(ctx, `UPDATE backup_run SET offsite_status = 'done', offsite_error = NULL WHERE id = $1`, run)
	require.NoError(t, err)
	disk.free = 50 << 30
	require.NoError(t, w.Check(ctx))
	require.Len(t, box.got, 3)
	assert.Contains(t, box.got[2].Body, "Over now:")
	assert.Contains(t, box.got[2].Body, "rclone: access denied")
	assert.Equal(t, "HiringPlug: 2 fixed, 1 still to do on the server", box.got[2].Subject, "a task still failed in the last day")
	require.NoError(t, w.Check(ctx))
	assert.Len(t, box.got, 3)
}

func TestABackupThatFailedOrStoppedIsAProblem(t *testing.T) {
	ctx := context.Background()
	admin := testutil.Current.Admin
	clock := time.Now()
	checks := alerts.Checks{DB: testutil.Current.App, Now: func() time.Time { return clock }}
	var run int64
	require.NoError(t, admin.QueryRow(ctx, `INSERT INTO backup_run (name, started_at, status, error)
		VALUES ('hiringplug-20261116-020000', now() + interval '1 hour', 'failed', 'pg_dump: disk full') RETURNING id`).Scan(&run))
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DELETE FROM backup_run WHERE id = $1`, run) })
	_, err := admin.Exec(ctx, `UPDATE backup_control SET auto = true, service_seen_at = now() - interval '1 hour'`)
	require.NoError(t, err)
	ps, err := checks.Problems(ctx)
	require.NoError(t, err)
	var texts []string
	for _, p := range ps {
		texts = append(texts, p.Key+": "+p.Text)
	}
	joined := strings.Join(texts, "\n")
	assert.Contains(t, joined, "backup-failed: The last backup failed")
	assert.Contains(t, joined, "pg_dump: disk full")
	assert.Contains(t, joined, "backup-service: The backup service has not checked in")

	// A backup that hangs is a problem too, though the service still checks in.
	_, err = admin.Exec(ctx, `UPDATE backup_run SET status = 'running', error = NULL, started_at = now() - interval '7 hours' WHERE id = $1`, run)
	require.NoError(t, err)
	_, err = admin.Exec(ctx, `UPDATE backup_control SET service_seen_at = now()`)
	require.NoError(t, err)
	ps, err = checks.Problems(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, ps)
	assert.Equal(t, "backup-stuck", ps[0].Key)
	assert.Contains(t, ps[0].Text, "has not finished")
	_, err = admin.Exec(ctx, `UPDATE backup_run SET started_at = now() - interval '1 hour' WHERE id = $1`, run)
	require.NoError(t, err)
	ps, err = checks.Problems(ctx)
	require.NoError(t, err)
	for _, p := range ps {
		assert.NotEqual(t, "backup-stuck", p.Key, "one running for an hour is just running")
	}

	// A laptop without the backup service: nothing to say about it.
	_, err = admin.Exec(ctx, `UPDATE backup_control SET service_seen_at = NULL`)
	require.NoError(t, err)
	ps, err = checks.Problems(ctx)
	require.NoError(t, err)
	for _, p := range ps {
		assert.NotEqual(t, "backup-service", p.Key)
	}
}
