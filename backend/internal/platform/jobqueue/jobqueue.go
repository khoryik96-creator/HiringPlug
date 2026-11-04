// Package jobqueue is the Go side of the job queue shared with the Python AI
// worker. The queue itself lives in PostgreSQL (migration 00003): both
// languages only call its SQL functions, so the rules are written once.
//
// Every job kind is declared in the API contract (x-jobs): its queue ("core"
// for the Go worker, "ai" for the Python worker) and its payload schema.
// Enqueue checks the payload against that schema, so a malformed job is a
// bug caught where it is created, not a mystery in another process.
package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/requestid"
)

// Queues.
const (
	QueueCore = "core" // handled by the Go worker (cmd/worker)
	QueueAI   = "ai"   // handled by the Python AI worker (ai/)
)

// Job is a job claimed by a worker.
type Job struct {
	ID          string
	TenantID    string
	Queue       string
	Kind        api.JobKind
	Payload     json.RawMessage
	Attempts    int
	MaxAttempts int
	RequestID   string
}

// Decode parses the payload into v (the generated type for the job kind).
func (j Job) Decode(v any) error {
	if err := json.Unmarshal(j.Payload, v); err != nil {
		return fmt.Errorf("job %s (%s): bad payload: %w", j.ID, j.Kind, err)
	}
	return nil
}

// Handler does a job's work inside tx, a transaction scoped to the job's
// tenant. The job is marked done in the same transaction, so its effects and
// its completion are committed together or not at all.
type Handler func(ctx context.Context, tx pgx.Tx, job Job) error

// Handlers maps each job kind to the code that runs it.
type Handlers map[api.JobKind]Handler

// Enqueue queues a job inside tx: it only exists if tx commits. payload must
// be the generated type for kind (e.g. api.DocumentExtractTextJob).
func Enqueue(ctx context.Context, tx pgx.Tx, tenantID string, kind api.JobKind, payload any) (string, error) {
	return EnqueueAt(ctx, tx, tenantID, kind, payload, time.Time{})
}

// EnqueueAt queues a job inside tx that runs no earlier than runAfter (now
// when zero), e.g. a reminder the day before an interview.
func EnqueueAt(ctx context.Context, tx pgx.Tx, tenantID string, kind api.JobKind, payload any, runAfter time.Time) (string, error) {
	info, ok := Kinds()[kind]
	if !ok {
		return "", fmt.Errorf("jobqueue: job kind %q is not declared in the contract (x-jobs)", kind)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("jobqueue: %s payload: %w", kind, err)
	}
	if err := validatePayload(info.Payload, data); err != nil {
		return "", fmt.Errorf("jobqueue: %s payload does not match schema %s: %w", kind, info.Payload, err)
	}
	var reqID *string
	if id := requestid.From(ctx); id != "" {
		reqID = &id
	}
	var id string
	var after *time.Time
	if !runAfter.IsZero() {
		after = &runAfter
	}
	err = tx.QueryRow(ctx, `SELECT job_enqueue($1, $2, $3, $4::jsonb, $5, coalesce($6, now()))`,
		tenantID, info.Queue, string(kind), string(data), reqID, after).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("jobqueue: enqueue %s: %w", kind, err)
	}
	return id, nil
}

// Worker claims jobs from one queue and runs their handlers.
type Worker struct {
	Pool     *pgxpool.Pool
	Queue    string
	ID       string // unique per process, e.g. hostname:pid
	Handlers Handlers
	Logger   *slog.Logger
	Lease    time.Duration // how long a claimed job is ours before others may retry it
	Idle     time.Duration // how long to wait for new jobs when the queue is empty
	// Tenant, if set, limits the worker to one company's jobs (tests: each
	// test's worker runs only its own companies' jobs). Empty: every company.
	Tenant string
}

var errLeaseLost = errors.New("lease lost: another worker took this job")

// Run processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.checkHandlers(); err != nil {
		return err
	}
	w.Logger.Info("worker started", "queue", w.Queue, "worker_id", w.ID)
	wake := w.listen(ctx)
	for ctx.Err() == nil {
		processed, err := w.RunOnce(ctx)
		if err != nil {
			w.Logger.Error("worker loop error", "error", err)
		}
		if processed && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
		case <-wake:
		case <-time.After(w.Idle):
		}
	}
	w.Logger.Info("worker stopped", "queue", w.Queue, "worker_id", w.ID)
	return nil
}

// RunOnce claims and processes at most one job. It reports whether a job was
// found. Tests call it directly to run the queue step by step.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, found, err := w.claim(ctx)
	if err != nil || !found {
		return false, err
	}
	ctx = requestid.With(ctx, job.RequestID)
	ctx = logx.Into(ctx, w.Logger.With("job_id", job.ID, "job_kind", string(job.Kind),
		"tenant_id", job.TenantID, "request_id", job.RequestID, "attempt", job.Attempts))
	start := time.Now()

	err = w.handle(ctx, job)
	log := logx.From(ctx)
	switch {
	case err == nil:
		log.Info("job done", "duration_ms", time.Since(start).Milliseconds())
	case errors.Is(err, errLeaseLost):
		log.Warn("job outlived its lease; its work was rolled back and another worker owns it")
	default:
		status, failErr := w.fail(ctx, job, err)
		if failErr != nil {
			return true, fmt.Errorf("recording failure of job %s: %w (job error: %w)", job.ID, failErr, err)
		}
		log.Error("job failed", "error", err, "new_status", status, "max_attempts", job.MaxAttempts)
	}
	return true, nil
}

func (w *Worker) handle(ctx context.Context, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	handler, ok := w.Handlers[job.Kind]
	if !ok {
		return fmt.Errorf("no handler for job kind %q on queue %q", job.Kind, w.Queue)
	}
	return database.InTx(ctx, w.Pool, authz.System(job.TenantID), func(tx pgx.Tx) error {
		if err := handler(ctx, tx, job); err != nil {
			return err
		}
		var completed bool
		if err := tx.QueryRow(ctx, `SELECT job_complete($1, $2)`, job.ID, w.ID).Scan(&completed); err != nil {
			return err
		}
		if !completed {
			return errLeaseLost
		}
		return nil
	})
}

const (
	claimAny       = `SELECT id, tenant_id, queue, kind, payload, attempts, max_attempts, request_id FROM job_claim($1, $2, $3)`
	claimForTenant = `SELECT id, tenant_id, queue, kind, payload, attempts, max_attempts, request_id FROM job_claim_for_tenant($1, $2, $3, $4)`
)

func (w *Worker) claim(ctx context.Context) (Job, bool, error) {
	var j Job
	var kind string
	var reqID *string
	query, args := claimAny, []any{w.Queue, w.ID, int(w.Lease.Seconds())}
	if w.Tenant != "" {
		query, args = claimForTenant, append(args, w.Tenant)
	}
	err := w.Pool.QueryRow(ctx, query, args...).Scan(&j.ID, &j.TenantID, &j.Queue, &kind, &j.Payload, &j.Attempts, &j.MaxAttempts, &reqID)
	if database.IsNoRows(err) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("claiming a job: %w", err)
	}
	j.Kind = api.JobKind(kind)
	if reqID != nil {
		j.RequestID = *reqID
	}
	return j, true, nil
}

func (w *Worker) fail(ctx context.Context, job Job, jobErr error) (string, error) {
	// Record the failure even if we are shutting down.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	var status string
	// job_fail also schedules the retry (growing waits), the same for every worker language.
	err := w.Pool.QueryRow(ctx, `SELECT job_fail($1, $2, $3)`, job.ID, w.ID, jobErr.Error()).Scan(&status)
	return status, err
}

// listen wakes the loop as soon as a job is queued (Postgres NOTIFY), instead
// of waiting for the next poll. Polling still runs as a fallback.
func (w *Worker) listen(ctx context.Context) <-chan struct{} {
	wake := make(chan struct{}, 1)
	go func() {
		for ctx.Err() == nil {
			if err := w.waitForNotifications(ctx, wake); err != nil && ctx.Err() == nil {
				w.Logger.Warn("job notifications unavailable; polling only", "error", err)
				select {
				case <-ctx.Done():
				case <-time.After(10 * time.Second):
				}
			}
		}
	}()
	return wake
}

func (w *Worker) waitForNotifications(ctx context.Context, wake chan<- struct{}) error {
	conn, err := w.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{"job_" + w.Queue}.Sanitize()); err != nil {
		return err
	}
	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err
		}
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

// checkHandlers refuses to start if a job kind on this queue has no handler.
func (w *Worker) checkHandlers() error {
	for _, kind := range KindsForQueue(w.Queue) {
		if _, ok := w.Handlers[kind]; !ok {
			return fmt.Errorf("jobqueue: no handler registered for %q (queue %q); wire it in internal/app", kind, w.Queue)
		}
	}
	return nil
}

// KindInfo is what the contract declares about a job kind.
type KindInfo struct {
	Module  string `json:"module"`
	Queue   string `json:"queue"`
	Payload string `json:"payload"`
}

// Kinds returns every job kind declared in the contract.
func Kinds() map[api.JobKind]KindInfo { return kinds() }

// KindsForQueue returns the job kinds handled on queue.
func KindsForQueue(queue string) []api.JobKind {
	var out []api.JobKind
	for kind, info := range kinds() {
		if info.Queue == queue {
			out = append(out, kind)
		}
	}
	return out
}

var kinds = sync.OnceValue(func() map[api.JobKind]KindInfo {
	spec, err := api.Spec()
	if err != nil {
		panic(fmt.Sprintf("jobqueue: embedded API contract does not load: %v", err))
	}
	raw, err := json.Marshal(spec.Extensions["x-jobs"])
	if err != nil {
		panic(fmt.Sprintf("jobqueue: x-jobs: %v", err))
	}
	var m map[api.JobKind]KindInfo
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("jobqueue: x-jobs: %v", err))
	}
	return m
})

func validatePayload(schemaName string, data []byte) error {
	spec, err := api.Spec()
	if err != nil {
		return err
	}
	ref, ok := spec.Components.Schemas[schemaName]
	if !ok || ref.Value == nil {
		return fmt.Errorf("schema %q not found", schemaName)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return ref.Value.VisitJSON(value)
}
