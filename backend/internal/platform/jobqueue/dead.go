package jobqueue

// Jobs that fail for good on the AI queue. The Python worker records the
// failure only; the database then queues a core notice (background_job.ended,
// migration 00199) in that same transaction, and the Go worker runs the
// kind's dead-job handler (Worker.OnDead) here, so what the job was for (a
// CV being read) does not wait forever. Also: whether a company already has
// a job under way for a record, so asking again never queues a second one.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// IsWorkerKind reports whether the Go worker handles kind itself (the
// database's notice of a dead job), with no module's handler.
func IsWorkerKind(kind api.JobKind) bool { return kind == api.JobKindBackgroundJobEnded }

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
// has a job of one of kinds waiting, running or held after a restore (it
// may still run) whose payload has key = value, such as a CV's reading by
// its documentId.
func Active(ctx context.Context, db Querier, kinds []api.JobKind, key, value string) (bool, error) {
	var active bool
	err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM job WHERE tenant_id = app_current_tenant()
		AND status IN ('pending', 'running', 'held') AND kind = ANY ($1) AND payload->>$2 = $3)`, names(kinds), key, value).Scan(&active)
	return active, err
}

// ActiveFor tells, for each of values (such as documentIds), how far its job
// of one of kinds is in the caller's company: "running", "waiting" (pending),
// or "held" (after a restore); values with none are left out.
func ActiveFor(ctx context.Context, db Querier, kinds []api.JobKind, key string, values []string) (map[string]string, error) {
	out := map[string]string{}
	if len(values) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `SELECT payload->>$2, CASE min(CASE status WHEN 'running' THEN 1 WHEN 'pending' THEN 2 ELSE 3 END)
		  WHEN 1 THEN 'running' WHEN 2 THEN 'waiting' ELSE 'held' END
		FROM job WHERE tenant_id = app_current_tenant() AND status IN ('pending', 'running', 'held')
		  AND kind = ANY ($1) AND payload->>$2 = ANY ($3)
		GROUP BY 1`, names(kinds), key, values)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value, state string
		if err := rows.Scan(&value, &state); err != nil {
			return nil, err
		}
		out[value] = state
	}
	return out, rows.Err()
}

// ContinueChain queues the next run of a repeating job that ended without
// running (cancelled after a restore, or stopped for good): each run queues
// the next, so its chain would end there. The next run is the dead job's own
// kind and payload, due at; nothing is queued when a run of the same chain
// (the same key = value, such as a mailboxId) is already waiting, running or
// held, so a chain has one next run however often this is called. It
// reports whether it queued one.
func ContinueChain(ctx context.Context, tx pgx.Tx, dead Job, key, value string, at time.Time) (bool, error) {
	// One at a time per chain, so two notices of one death cannot both queue a run.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "chain:"+string(dead.Kind)+"/"+key+"="+value); err != nil {
		return false, err
	}
	if active, err := Active(ctx, tx, []api.JobKind{dead.Kind}, key, value); err != nil || active {
		return false, err
	}
	_, err := EnqueueAt(ctx, tx, dead.TenantID, dead.Kind, dead.Payload, at)
	return err == nil, err
}
