package jobqueue

// After a restore: no external actions until someone says so. A backup
// holds the job table as it was, so the emails, calendar updates, webhooks
// and automations waiting in it would run again: on another server (a
// staging copy of production must never email real candidates), or on this
// one after they had already happened. `hp restore` holds them (status
// held: no worker claims them) and makes interrupted local work ready
// again; Settings > System lists the held jobs, to release or cancel.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Outbound are the kinds held after a restore: they act outside HiringPlug
// (send, write to another system, spend money), or read another system with
// the restored sign-ins and act on what they find. Everything else is
// local (reading CVs, PDF copies, search by meaning, saving results) and
// runs again safely. A new kind that acts outside belongs here.
var Outbound = []api.JobKind{
	// Writes outside: emails, calendars, other systems, money.
	api.JobKindEmailSend, api.JobKindEmailInterviewReminder, api.JobKindEmailOutlookScheduled,
	api.JobKindInterviewRemind, api.JobKindNotificationsDigest, api.JobKindPlacementInvoice,
	api.JobKindAutomationRun, api.JobKindAutomationEvent,
	api.JobKindCalendarPush, api.JobKindCalendarSourceChanged, api.JobKindWebhookDeliver,
	api.JobKindClaimUpload, api.JobKindClaimRemoveUpload,
	api.JobKindContactApolloLookup, api.JobKindContactCheckMailbox,
	// Reads outside with the restored sign-ins (two servers refreshing one
	// mailbox's sign-in can end it for both) and act on what they find
	// (mail filed, leads added, automations started).
	api.JobKindEmailOutlookSync, api.JobKindEmailBounceScan, api.JobKindCalendarRefresh,
	api.JobKindSchedulingRefreshCalendar, api.JobKindLeadSourceCheck, api.JobKindCompanyAutofill,
}

// HeldCount is how many jobs of one kind a restore held.
type HeldCount struct {
	Kind  string
	Count int
}

// AfterRestore is what a restore did with the jobs it put back.
type AfterRestore struct {
	Held  []HeldCount
	Reset int // interrupted jobs made ready again
}

// HeldTotal is how many jobs were held.
func (a AfterRestore) HeldTotal() int {
	n := 0
	for _, h := range a.Held {
		n += h.Count
	}
	return n
}

// Restored is the server's last restore (restore_marker).
type Restored struct {
	BackupName        string
	SourceServer      *string // nil: a backup too old to say
	DestinationServer string
	RestoredAt        time.Time
	Held              int
	Resumed           bool
	ReleasedAt        *time.Time
}

// FromAnotherServer reports whether the backup was made on another
// database server (or does not say which).
func (r Restored) FromAnotherServer() bool {
	return r.SourceServer == nil || *r.SourceServer != r.DestinationServer
}

// Holding reports whether jobs it held still wait for someone.
func (r Restored) Holding() bool { return r.ReleasedAt == nil }

// HoldAfterRestore records the restore and, unless resume, holds the
// outbound jobs; jobs still marked running (their worker is gone) are made
// ready again. One transaction, as the database owner, before the workers
// start.
func HoldAfterRestore(ctx context.Context, tx pgx.Tx, r Restored) (AfterRestore, error) {
	var out AfterRestore
	if !r.Resumed {
		kinds := make([]string, len(Outbound))
		for i, k := range Outbound {
			kinds[i] = string(k)
		}
		rows, err := tx.Query(ctx, `SELECT kind, held FROM job_hold($1)`, kinds)
		if err != nil {
			return out, fmt.Errorf("holding the outbound jobs: %w", err)
		}
		if out.Held, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (HeldCount, error) {
			var h HeldCount
			err := row.Scan(&h.Kind, &h.Count)
			return h, err
		}); err != nil {
			return out, fmt.Errorf("holding the outbound jobs: %w", err)
		}
	}
	if err := tx.QueryRow(ctx, `SELECT job_reset_leases()`).Scan(&out.Reset); err != nil {
		return out, fmt.Errorf("making interrupted jobs ready: %w", err)
	}
	released := any(nil)
	if out.HeldTotal() == 0 {
		released = time.Now()
	}
	_, err := tx.Exec(ctx, `INSERT INTO restore_marker (id, backup_name, source_server, destination_server, restored_at, held, resumed, released_at)
		VALUES (true, $1, $2, $3, now(), $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET backup_name = EXCLUDED.backup_name, source_server = EXCLUDED.source_server,
		  destination_server = EXCLUDED.destination_server, restored_at = EXCLUDED.restored_at, held = EXCLUDED.held,
		  resumed = EXCLUDED.resumed, released_at = EXCLUDED.released_at`,
		r.BackupName, r.SourceServer, r.DestinationServer, out.HeldTotal(), r.Resumed, released)
	if err != nil {
		return out, fmt.Errorf("recording the restore: %w", err)
	}
	return out, nil
}

// LastRestore returns the server's last restore; ok is false when it was
// never restored (or before restores were recorded).
func LastRestore(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}) (r Restored, ok bool, err error) {
	err = db.QueryRow(ctx, `SELECT backup_name, source_server, destination_server, restored_at, held, resumed, released_at
		FROM restore_marker`).Scan(&r.BackupName, &r.SourceServer, &r.DestinationServer, &r.RestoredAt, &r.Held, &r.Resumed, &r.ReleasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Restored{}, false, nil
	}
	return r, err == nil, err
}

// HeldJob is a job a restore held.
type HeldJob struct {
	ID, Kind  string
	Attempts  int
	CreatedAt time.Time
	HeldAt    *time.Time
	RunAfter  time.Time
}

// Held returns a company's held jobs, oldest first.
func Held(ctx context.Context, db Querier, tenantID string, limit int) ([]HeldJob, error) {
	rows, err := db.Query(ctx, `SELECT id, kind, attempts, created_at, held_at, run_after FROM job_held($1, $2)`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (HeldJob, error) {
		var h HeldJob
		err := r.Scan(&h.ID, &h.Kind, &h.Attempts, &h.CreatedAt, &h.HeldAt, &h.RunAfter)
		return h, err
	})
}

// Release lets one of a company's held jobs run (jobID), or all of them
// (""). It returns how many.
func Release(ctx context.Context, db Querier, tenantID, jobID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT job_release($1, $2)`, tenantID, nullIfEmpty(jobID)).Scan(&n)
	return n, err
}

// CancelHeld cancels one of a company's held jobs (jobID), or all of them
// (""): dead, never run. It returns how many.
func CancelHeld(ctx context.Context, db Querier, tenantID, jobID string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT job_cancel_held($1, $2)`, tenantID, nullIfEmpty(jobID)).Scan(&n)
	return n, err
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
