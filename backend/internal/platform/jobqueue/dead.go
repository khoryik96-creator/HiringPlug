package jobqueue

// Jobs that fail for good on the AI queue. The Python worker records the
// failure only; the database then queues a core notice (background_job.ended,
// migration 00199) in that same transaction, and the Go worker runs the
// kind's dead-job handler (Worker.OnDead) here, so what the job was for (a
// CV being read) does not wait forever. Also: whether a company already has
// a job under way for a record, so asking again never queues a second one.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// jobEnded runs the dead-job handler of the job a notice names,
// with that job as it ended (its payload and last error). Nothing to do when
// its kind has none, or when the job is no longer dead (tried again since)
// or gone (pruned).
func (w *Worker) jobEnded(ctx context.Context, tx pgx.Tx, notice Job) error {
	var n api.BackgroundJobEndedJob
	if err := notice.Decode(&n); err != nil {
		return err
	}
	handler := w.OnDead[api.JobKind(n.Kind)]
	if handler == nil {
		return nil
	}
	var dead Job
	var kind string
	err := tx.QueryRow(ctx, `SELECT id, tenant_id, queue, kind, payload, attempts, max_attempts,
		coalesce(request_id, ''), coalesce(last_error, '')
		FROM job WHERE id = $1 AND tenant_id = $2 AND status = 'dead'`, n.JobId, notice.TenantID).
		Scan(&dead.ID, &dead.TenantID, &dead.Queue, &kind, &dead.Payload, &dead.Attempts, &dead.MaxAttempts, &dead.RequestID, &dead.LastError)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dead.Kind = api.JobKind(kind)
	return handler(ctx, tx, dead)
}

// Active reports whether the caller's company (the transaction's tenant)
// has a job of one of kinds waiting or running whose payload has key =
// value, such as a CV's reading by its documentId.
func Active(ctx context.Context, db Querier, kinds []api.JobKind, key, value string) (bool, error) {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	var active bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job WHERE tenant_id = app_current_tenant()
		AND status IN ('pending', 'running') AND kind = ANY ($1) AND payload->>$2 = $3)`, names, key, value).Scan(&active)
	return active, err
}
