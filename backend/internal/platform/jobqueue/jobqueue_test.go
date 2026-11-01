package jobqueue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

func validPayload() api.DocumentTextExtractedJob {
	return api.DocumentTextExtractedJob{
		DocumentId: "doc_01jbz6w0m0k8q9x2r3s4t5v6w7", Status: api.ExtractionStatusFailed, Extractor: "test@1",
		Text: nullable.NewNullNullable[string](), PageCount: nullable.NewNullNullable[int](),
		ErrorMessage: nullable.NewNullableWithValue("unreadable"), Warnings: []string{},
	}
}

func enqueue(t *testing.T, tenantID string, payload any) (string, error) {
	t.Helper()
	ctx := context.Background()
	var id string
	err := database.InTx(ctx, testutil.Current.App, authz.System(tenantID), func(tx pgx.Tx) error {
		var err error
		id, err = jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentTextExtracted, payload)
		return err
	})
	return id, err
}

func worker(handler jobqueue.Handler) *jobqueue.Worker {
	return &jobqueue.Worker{
		Pool: testutil.Current.App, Queue: jobqueue.QueueCore, ID: "test-worker",
		Handlers: jobqueue.Handlers{api.JobKindDocumentTextExtracted: handler},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)), Lease: time.Minute, Idle: 10 * time.Millisecond,
	}
}

type jobState struct {
	Status    string
	Attempts  int
	LastError *string
}

func state(t *testing.T, id string) jobState {
	t.Helper()
	var s jobState
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(),
		`SELECT status, attempts, last_error FROM job WHERE id = $1`, id).Scan(&s.Status, &s.Attempts, &s.LastError))
	return s
}

// runUntil processes jobs until the job reaches want (other tests' jobs may
// be processed along the way; their handlers are the same test handler).
func runUntil(t *testing.T, w *jobqueue.Worker, id, want string) {
	t.Helper()
	for range 50 {
		_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE job SET run_after = now() WHERE id = $1 AND status = 'pending'`, id)
		require.NoError(t, err)
		_, err = w.RunOnce(context.Background())
		require.NoError(t, err)
		if state(t, id).Status == want {
			return
		}
	}
	t.Fatalf("job %s never reached %s (now %+v)", id, want, state(t, id))
}

func TestAJobThatSucceedsIsDone(t *testing.T) {
	tenant := testutil.Current.NewTenant(t)
	id, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	var sawTenant string
	runUntil(t, worker(func(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
		if job.ID == id {
			return tx.QueryRow(ctx, `SELECT app_current_tenant()`).Scan(&sawTenant)
		}
		return nil
	}), id, "done")
	assert.Equal(t, tenant.ID, sawTenant, "handlers run scoped to the job's tenant")
}

func TestAFailingJobIsRetriedThenMarkedDead(t *testing.T) {
	tenant := testutil.Current.NewTenant(t)
	id, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(context.Background(), `UPDATE job SET max_attempts = 2 WHERE id = $1`, id)
	require.NoError(t, err)

	w := worker(func(context.Context, pgx.Tx, jobqueue.Job) error { return errors.New("provider timed out") })
	_, err = w.RunOnce(context.Background())
	require.NoError(t, err)
	s := state(t, id)
	assert.Equal(t, "pending", s.Status, "first failure: retried later")
	assert.Equal(t, 1, s.Attempts)
	require.NotNil(t, s.LastError)
	assert.Contains(t, *s.LastError, "provider timed out")

	runUntil(t, w, id, "dead")
	assert.Equal(t, 2, state(t, id).Attempts)
}

func TestAPanickingHandlerDoesNotKillTheWorker(t *testing.T) {
	tenant := testutil.Current.NewTenant(t)
	id, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	w := worker(func(context.Context, pgx.Tx, jobqueue.Job) error { panic("boom") })
	_, err = w.RunOnce(context.Background())
	require.NoError(t, err)
	s := state(t, id)
	assert.Equal(t, "pending", s.Status)
	assert.Contains(t, *s.LastError, "panic: boom")
}

func TestPayloadsAreCheckedAgainstTheContract(t *testing.T) {
	tenant := testutil.Current.NewTenant(t)
	_, err := enqueue(t, tenant.ID, map[string]any{"documentId": "not-an-id"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match schema DocumentTextExtractedJob")

	err = database.InTx(context.Background(), testutil.Current.App, authz.System(tenant.ID), func(tx pgx.Tx) error {
		_, err := jobqueue.Enqueue(context.Background(), tx, tenant.ID, "made.up", map[string]any{})
		return err
	})
	assert.ErrorContains(t, err, "not declared in the contract")
}

func TestAnExpiredLeaseLetsAnotherWorkerTakeOver(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	id, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	// Worker A claims the job, then "dies": its lease is made to expire.
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() - interval '1 hour' WHERE id = $1`, id)
	require.NoError(t, err)
	var claimed string
	require.NoError(t, testutil.Current.App.QueryRow(ctx, `SELECT id FROM job_claim('core', 'worker-a', 60)`).Scan(&claimed))
	require.Equal(t, id, claimed)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET locked_until = now() - interval '1 second' WHERE id = $1`, id)
	require.NoError(t, err)

	runUntil(t, worker(func(context.Context, pgx.Tx, jobqueue.Job) error { return nil }), id, "done")

	var completedByA bool
	require.NoError(t, testutil.Current.App.QueryRow(ctx, `SELECT job_complete($1, 'worker-a')`, id).Scan(&completedByA))
	assert.False(t, completedByA, "the worker that lost its lease cannot complete the job")
}

func TestTheWorkerRefusesToStartWithAMissingHandler(t *testing.T) {
	w := worker(nil)
	w.Handlers = jobqueue.Handlers{}
	err := w.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no handler registered")
}

func TestRetriesWaitLongerEachTime(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	id, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET max_attempts = 50, run_after = now() - interval '1 hour' WHERE id = $1`, id)
	require.NoError(t, err)

	waitAfterAttempt := func(attempts int) time.Duration {
		_, err := testutil.Current.Admin.Exec(ctx,
			`UPDATE job SET status = 'running', locked_by = 'w', attempts = $2 WHERE id = $1`, id, attempts)
		require.NoError(t, err)
		var status string
		require.NoError(t, testutil.Current.App.QueryRow(ctx, `SELECT job_fail($1, 'w', 'boom')`, id).Scan(&status))
		var seconds float64
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
			`SELECT extract(epoch FROM run_after - now()) FROM job WHERE id = $1`, id).Scan(&seconds))
		return time.Duration(seconds * float64(time.Second))
	}
	assert.InDelta(t, 10, waitAfterAttempt(1).Seconds(), 2)
	assert.InDelta(t, 20, waitAfterAttempt(2).Seconds(), 2)
	assert.InDelta(t, 40, waitAfterAttempt(3).Seconds(), 2)
	assert.InDelta(t, 2560, waitAfterAttempt(9).Seconds(), 2)
	assert.InDelta(t, 3600, waitAfterAttempt(12).Seconds(), 2, "capped at one hour")
}
