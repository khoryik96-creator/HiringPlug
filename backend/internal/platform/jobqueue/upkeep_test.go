package jobqueue_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestEachJobKindHasThePriorityTheContractGivesIt(t *testing.T) {
	k := jobqueue.Kinds()
	assert.Equal(t, 1, k[api.JobKindEmailSend].Rank(), "someone is waiting for it")
	assert.Equal(t, 1, k[api.JobKindAssistantAsk].Rank())
	assert.Equal(t, 5, k[api.JobKindDocumentTextExtracted].Rank())
	assert.Equal(t, 9, k[api.JobKindDocumentRenderPdf].Rank(), "a preview can wait")
	assert.Equal(t, 9, k[api.JobKindWebhookDeliver].Rank())
}

func TestUrgentJobsAreTakenFirst(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	normal, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	var urgent string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT job_enqueue($1, 'core', 'document.text_extracted', $2::jsonb, p_priority => 1::smallint)`,
		tenant.ID, mustJSON(t, validPayload())).Scan(&urgent))
	var p int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT priority FROM job WHERE id = $1`, normal).Scan(&p))
	assert.Equal(t, 5, p, "Enqueue stores the kind's priority")

	var order []string
	w := worker(func(_ context.Context, _ pgx.Tx, job jobqueue.Job) error {
		order = append(order, job.ID)
		return nil
	})
	w.Tenant = tenant.ID
	for range 2 {
		_, err := w.RunOnce(ctx)
		require.NoError(t, err)
	}
	assert.Equal(t, []string{urgent, normal}, order, "queued later, but someone is waiting for it")
}

func TestAFinishedJobKeepsNoneOfItsPayload(t *testing.T) {
	tenant := testutil.Current.NewTenant(t)
	id, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	w := worker(func(context.Context, pgx.Tx, jobqueue.Job) error { return nil })
	w.Tenant = tenant.ID
	runUntil(t, w, id, "done")
	var payload string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT payload::text FROM job WHERE id = $1`, id).Scan(&payload))
	assert.Equal(t, "{}", payload, "CV text and the like do not stay in the queue")
}

func TestOldFinishedAndDeadJobsArePruned(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	ids := make([]string, 4)
	for i := range ids {
		var err error
		ids[i], err = enqueue(t, tenant.ID, validPayload())
		require.NoError(t, err)
	}
	set := func(id, sql string) {
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET `+sql+` WHERE id = $1`, id)
		require.NoError(t, err)
	}
	set(ids[0], `status = 'done', finished_at = now() - interval '31 days'`)
	set(ids[1], `status = 'dead', finished_at = now() - interval '31 days'`)
	set(ids[2], `status = 'done', finished_at = now() - interval '29 days'`)
	// ids[3] is still waiting.
	_, err := jobqueue.Prune(ctx, testutil.Current.App, jobqueue.KeepFinished)
	require.NoError(t, err)
	var left []string
	rows, err := testutil.Current.Admin.Query(ctx, `SELECT id FROM job WHERE tenant_id = $1 ORDER BY id`, tenant.ID)
	require.NoError(t, err)
	left, err = pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{ids[2], ids[3]}, left)
}

func TestACompanySeesItsBackgroundJobsAndTriesADeadOneAgain(t *testing.T) {
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	waiting, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	dead, err := enqueue(t, tenant.ID, validPayload())
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET status = 'dead', attempts = 5, last_error = 'smtp: refused', finished_at = now() WHERE id = $1`, dead)
	require.NoError(t, err)

	summary, err := jobqueue.Summary(ctx, testutil.Current.App, tenant.ID)
	require.NoError(t, err)
	require.Len(t, summary, 1)
	assert.Equal(t, string(api.JobKindDocumentTextExtracted), summary[0].Kind)
	assert.Equal(t, 1, summary[0].Waiting)
	assert.Equal(t, 1, summary[0].Dead)
	require.NotNil(t, summary[0].OldestWaiting)
	list, err := jobqueue.Dead(ctx, testutil.Current.App, tenant.ID, 50)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, jobqueue.DeadJob{ID: dead, Kind: string(api.JobKindDocumentTextExtracted), LastError: "smtp: refused", Attempts: 5, FinishedAt: list[0].FinishedAt}, list[0])

	other := testutil.Current.NewTenant(t)
	ok, err := jobqueue.Retry(ctx, testutil.Current.App, other.ID, dead)
	require.NoError(t, err)
	assert.False(t, ok, "another company cannot touch it")
	ok, err = jobqueue.Retry(ctx, testutil.Current.App, tenant.ID, waiting)
	require.NoError(t, err)
	assert.False(t, ok, "only dead jobs are tried again")
	ok, err = jobqueue.Retry(ctx, testutil.Current.App, tenant.ID, dead)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, jobState{Status: "pending"}, state(t, dead))
}

func TestSeveralJobsRunAtOnce(t *testing.T) {
	tenant := testutil.Current.NewTenant(t)
	for range 2 {
		_, err := enqueue(t, tenant.ID, validPayload())
		require.NoError(t, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	started, both := 0, make(chan struct{})
	w := worker(nil)
	w.Handlers = jobqueue.Handlers{}
	for _, kind := range jobqueue.KindsForQueue(jobqueue.QueueCore) { // Run checks every kind has one
		w.Handlers[kind] = func(context.Context, pgx.Tx, jobqueue.Job) error { return nil }
	}
	w.Handlers[api.JobKindDocumentTextExtracted] = func(context.Context, pgx.Tx, jobqueue.Job) error {
		mu.Lock()
		started++
		if started == 2 {
			close(both)
		}
		mu.Unlock()
		select { // each job waits for the other: only possible side by side
		case <-both:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	w.Tenant, w.Concurrency = tenant.ID, 2
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case <-both:
	case <-ctx.Done():
		t.Fatal("the two jobs never ran at the same time")
	}
	cancel()
	require.NoError(t, <-done)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
