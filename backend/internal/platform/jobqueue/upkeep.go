package jobqueue

// Keeping the queue tidy, and each company's view of its background jobs
// (Settings > System). Finished jobs lose their payload when they complete
// (job_complete); Prune deletes them, and dead jobs, after a while.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// KeepFinished is how long finished and dead jobs stay (for "what happened"
// and for trying a dead job again).
const KeepFinished = 30 * 24 * time.Hour

// Querier runs SQL: a pool or a transaction.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Prune deletes jobs finished or dead for longer than keep. It returns how
// many were deleted.
func Prune(ctx context.Context, db Querier, keep time.Duration) (int, error) {
	total := 0
	for {
		var n int
		if err := db.QueryRow(ctx, `SELECT job_prune(make_interval(secs => $1))`, keep.Seconds()).Scan(&n); err != nil {
			return total, fmt.Errorf("pruning jobs: %w", err)
		}
		total += n
		if n < 5000 { // one batch at a time, so no long lock
			return total, nil
		}
	}
}

// KindStatus is a company's jobs of one kind.
type KindStatus struct {
	Kind                             string
	Waiting, Running, Retrying, Dead int
	OldestWaiting                    *time.Time
}

// DeadJob is a job that failed for good.
type DeadJob struct {
	ID, Kind, LastError string
	Attempts            int
	FinishedAt          time.Time
}

// Summary returns a company's background jobs by kind (dead ones within KeepFinished).
func Summary(ctx context.Context, db Querier, tenantID string) ([]KindStatus, error) {
	rows, err := db.Query(ctx, `SELECT kind, waiting, running, retrying, dead, oldest_waiting
		FROM job_summary($1, make_interval(secs => $2))`, tenantID, KeepFinished.Seconds())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (KindStatus, error) {
		var k KindStatus
		err := r.Scan(&k.Kind, &k.Waiting, &k.Running, &k.Retrying, &k.Dead, &k.OldestWaiting)
		return k, err
	})
}

// Dead returns a company's latest jobs that failed for good.
func Dead(ctx context.Context, db Querier, tenantID string, limit int) ([]DeadJob, error) {
	rows, err := db.Query(ctx, `SELECT id, kind, attempts, coalesce(last_error, ''), finished_at
		FROM job_dead($1, make_interval(secs => $2), $3)`, tenantID, KeepFinished.Seconds(), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (DeadJob, error) {
		var d DeadJob
		err := r.Scan(&d.ID, &d.Kind, &d.Attempts, &d.LastError, &d.FinishedAt)
		return d, err
	})
}

// Retry queues a company's dead job again from its first attempt. It reports
// false when the company has no such dead job.
func Retry(ctx context.Context, db Querier, tenantID, jobID string) (bool, error) {
	var ok bool
	err := db.QueryRow(ctx, `SELECT job_retry($1, $2)`, tenantID, jobID).Scan(&ok)
	return ok, err
}
