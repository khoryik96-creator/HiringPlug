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

// Review of 1.47.0 (before it): a claim took back a running job whose lease
// had run out whatever its attempts, so a job that kills its worker was
// claimed and killed it again, for ever. Every claim function now ends such
// a job on its last attempt (dead, saying why) instead, and its dead-job
// handler runs; one with tries left is taken back as before.
func TestAJobWhoseWorkerDiesOnItsLastAttemptEnds(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})
	expired := func(queue, kind string, attempts int) string {
		t.Helper()
		var id string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `INSERT INTO job (tenant_id, queue, kind, payload, status, attempts, max_attempts,
			locked_by, locked_until, run_after) VALUES ($1, $2, $3, '{"emailId":"eml_1"}', 'running', $4, 5, 'dead-worker', now() - interval '1 minute',
			now() - interval '1 hour') RETURNING id`, tenant.ID, queue, kind, attempts).Scan(&id))
		return id
	}
	for _, claim := range []string{
		`SELECT count(*) FROM job_claim('ai', 'w2', 60)`,
		`SELECT count(*) FROM job_claim_kinds('ai', 'w2', 60, ARRAY['document.extract_text'])`,
		`SELECT count(*) FROM job_claim_except_kinds('ai', 'w2', 60, ARRAY['meaning.read_query'])`,
		`SELECT count(*) FROM job_claim_for_tenant('ai', 'w2', 60, '` + tenant.ID + `')`,
	} {
		last := expired("ai", "document.extract_text", 5)
		var claimed int
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, claim).Scan(&claimed), claim)
		status, attempts := statusOf(t, last)
		assert.Equal(t, "dead", status, claim)
		assert.Equal(t, 5, attempts, "not claimed a sixth time: %s", claim)
		assert.Equal(t, 0, claimed, claim)
	}
	var reason string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT last_error FROM job WHERE tenant_id = $1 AND queue = 'ai' LIMIT 1`, tenant.ID).Scan(&reason))
	assert.Contains(t, reason, "lease ran out")

	again := expired("ai", "document.extract_text", 2)
	var claimed string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT id FROM job_claim_for_tenant('ai', 'w2', 60, $1)`, tenant.ID).Scan(&claimed))
	assert.Equal(t, again, claimed, "tries left: taken back")
	status, attempts := statusOf(t, again)
	assert.Equal(t, "running", status)
	assert.Equal(t, 3, attempts)

	// A core job too, and its dead-job handler runs (through the notice).
	email := expired("core", "email.send", 5)
	var got []string
	w := &jobqueue.Worker{Pool: testutil.Current.App, Queue: jobqueue.QueueCore, ID: "test-worker", Tenant: tenant.ID,
		Handlers: jobqueue.Handlers{api.JobKindEmailSend: func(context.Context, pgx.Tx, jobqueue.Job) error {
			t.Error("claimed a sixth time")
			return nil
		}},
		OnDead: jobqueue.Handlers{api.JobKindEmailSend: func(_ context.Context, _ pgx.Tx, j jobqueue.Job) error {
			got = append(got, j.ID)
			return nil
		}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Lease: time.Minute, Idle: 10 * time.Millisecond}
	for range 20 { // the notices of the AI jobs above too
		found, err := w.RunOnce(ctx)
		require.NoError(t, err)
		if !found {
			break
		}
	}
	status, _ = statusOf(t, email)
	assert.Equal(t, "dead", status)
	assert.Equal(t, []string{email}, got)
}
