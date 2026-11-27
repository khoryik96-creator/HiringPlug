package jobqueue_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// insertJob puts a job in the queue as a backup would bring it back.
func insertJob(t *testing.T, tenantID, queue, kind, status string) string {
	t.Helper()
	var id string
	lease := any(nil)
	if status == "running" {
		lease = time.Now().Add(-time.Hour) // its worker is gone
	}
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `INSERT INTO job (tenant_id, queue, kind, payload, status, attempts,
		locked_by, locked_until, finished_at, run_after) VALUES ($1, $2, $3, '{"documentId":"doc_01jbz6w0m0k8q9x2r3s4t5v6w7"}', $4,
		CASE WHEN $4 = 'pending' THEN 0 ELSE 1 END, CASE WHEN $4 = 'running' THEN 'old-server' END, $5,
		CASE WHEN $4 IN ('done', 'dead') THEN now() END, now() - interval '1 minute') RETURNING id`,
		tenantID, queue, kind, status, lease).Scan(&id))
	return id
}

func statusOf(t *testing.T, id string) (status string, attempts int) {
	t.Helper()
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT status, attempts FROM job WHERE id = $1`, id).Scan(&status, &attempts))
	return status, attempts
}

// IMP-002: a restore with jobs waiting, running, done and failed holds the
// ones that act outside HyPlug (whatever their state, waiting or
// running), makes interrupted local work ready again, leaves finished jobs
// alone, and no worker takes a held job until it is released.
func TestARestoreHoldsOutboundJobsAndResumesLocalOnes(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM restore_marker`)
	})
	email := insertJob(t, tenant.ID, "core", "email.send", "pending")
	webhook := insertJob(t, tenant.ID, "core", "webhook.deliver", "running")
	sync := insertJob(t, tenant.ID, "core", "email.outlook_sync", "pending")
	extract := insertJob(t, tenant.ID, "ai", "document.extract_text", "running")
	pdf := insertJob(t, tenant.ID, "ai", "document.render_pdf", "pending")
	sent := insertJob(t, tenant.ID, "core", "email.send", "done")
	failed := insertJob(t, tenant.ID, "core", "email.send", "dead")

	source := "7311000000000000001"
	tx, err := testutil.Current.Admin.Begin(ctx)
	require.NoError(t, err)
	after, err := jobqueue.HoldAfterRestore(ctx, tx, jobqueue.Restored{BackupName: "hiringplug-20261125-023050", SourceServer: &source, DestinationServer: "7420000000000000002"})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	assert.Equal(t, []jobqueue.HeldCount{{Kind: "email.outlook_sync", Count: 1}, {Kind: "email.send", Count: 1}, {Kind: "webhook.deliver", Count: 1}}, after.Held)
	assert.Equal(t, 1, after.Reset)

	for _, id := range []string{email, webhook, sync} {
		st, _ := statusOf(t, id)
		assert.Equal(t, "held", st, id)
	}
	st, attempts := statusOf(t, extract)
	assert.Equal(t, "pending", st, "interrupted local work runs again")
	assert.Zero(t, attempts, "the interrupted attempt does not count")
	st, _ = statusOf(t, pdf)
	assert.Equal(t, "pending", st)
	st, _ = statusOf(t, sent)
	assert.Equal(t, "done", st)
	st, _ = statusOf(t, failed)
	assert.Equal(t, "dead", st)

	r, ok, err := jobqueue.LastRestore(ctx, testutil.Current.App)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, r.FromAnotherServer())
	assert.True(t, r.Holding())
	assert.Equal(t, 3, r.Held)

	// No worker takes a held job.
	var claimed []string
	for {
		var id string
		err := testutil.Current.App.QueryRow(ctx, `SELECT id FROM job_claim_for_tenant('core', 'w', 60, $1)`, tenant.ID).Scan(&id)
		if err != nil {
			require.ErrorIs(t, err, pgx.ErrNoRows)
			break
		}
		claimed = append(claimed, id)
	}
	assert.Empty(t, claimed, "only held outbound jobs wait on the core queue")

	held, err := jobqueue.Held(ctx, testutil.Current.App, tenant.ID, 10)
	require.NoError(t, err)
	assert.Len(t, held, 3)
	other := testutil.Current.NewTenant(t)
	n, err := jobqueue.Release(ctx, testutil.Current.App, other.ID, email)
	require.NoError(t, err)
	assert.Zero(t, n, "another company cannot release it")

	n, err = jobqueue.Release(ctx, testutil.Current.App, tenant.ID, email)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	st, _ = statusOf(t, email)
	assert.Equal(t, "pending", st)
	n, err = jobqueue.CancelHeld(ctx, testutil.Current.App, tenant.ID, "")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	st, _ = statusOf(t, webhook)
	assert.Equal(t, "dead", st, "cancelled: never run")
	var notices int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'background_job.ended'`, tenant.ID).Scan(&notices))
	assert.Equal(t, 2, notices, "each cancelled job tells the Go worker, as one failing for good does")
	r, _, err = jobqueue.LastRestore(ctx, testutil.Current.App)
	require.NoError(t, err)
	assert.False(t, r.Holding(), "settled once nothing is held")
}

// Restored with -resume-outbound: nothing held, and nothing waits.
func TestARestoreAskedToResumeHoldsNothing(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM restore_marker`)
	})
	email := insertJob(t, tenant.ID, "core", "email.send", "pending")
	tx, err := testutil.Current.Admin.Begin(ctx)
	require.NoError(t, err)
	after, err := jobqueue.HoldAfterRestore(ctx, tx, jobqueue.Restored{BackupName: "hiringplug-20261125-023050", DestinationServer: "1", Resumed: true})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	assert.Zero(t, after.HeldTotal())
	st, _ := statusOf(t, email)
	assert.Equal(t, "pending", st)
	r, _, err := jobqueue.LastRestore(ctx, testutil.Current.App)
	require.NoError(t, err)
	assert.False(t, r.Holding())
	assert.True(t, r.FromAnotherServer(), "a backup that does not say where it was made counts as another server's")
}

// BUG-004: an AI-queue job that fails for good reaches the Go worker, which
// runs that kind's dead-job handler with the job as it ended.
func TestAnAIJobThatFailsForGoodRunsItsDeadJobHandler(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})
	id := insertJob(t, tenant.ID, "ai", "document.extract_text", "running")
	var status string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT job_give_up($1, 'old-server', 'DownloadGoneError: download failed with HTTP 404')`, id).Scan(&status))
	require.Equal(t, "dead", status)

	var got []jobqueue.Job
	w := &jobqueue.Worker{Pool: testutil.Current.App, Queue: jobqueue.QueueCore, ID: "test-worker", Tenant: tenant.ID,
		OnDead: jobqueue.Handlers{api.JobKindDocumentExtractText: func(_ context.Context, _ pgx.Tx, j jobqueue.Job) error {
			got = append(got, j)
			return nil
		}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Lease: time.Minute, Idle: 10 * time.Millisecond}
	found, err := w.RunOnce(ctx)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0].ID)
	assert.Equal(t, api.JobKindDocumentExtractText, got[0].Kind)
	assert.Equal(t, "DownloadGoneError: download failed with HTTP 404", got[0].LastError)
	assert.JSONEq(t, `{"documentId":"doc_01jbz6w0m0k8q9x2r3s4t5v6w7"}`, string(got[0].Payload))
	found, err = w.RunOnce(ctx)
	require.NoError(t, err)
	assert.False(t, found, "one notice per job")
}
