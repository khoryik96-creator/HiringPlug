package main

// After a restore: the database brought up to date, the restore recorded,
// and the jobs that act outside HiringPlug held until someone releases them
// (jobqueue.HoldAfterRestore).

import (
	"context"
	"fmt"
	"strings"

	"github.com/khoryik96-creator/hiringplug/backend/db"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/aipaid"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// settleJobs runs after the database and files are back. A failure exits
// non-zero: the workers must not start before the jobs are held.
func settleJobs(ctx context.Context, o backup.Options, name string, resume, resumeAI bool) error {
	// A backup from an older version lacks what holding needs (and the API
	// refuses a database behind it anyway).
	if err := database.Migrate(ctx, o.AdminURL, db.Migrations()); err != nil {
		return fmt.Errorf("restored %s, but bringing its database up to date failed; do not start the workers until `hp bootstrap` works and this restore is run again: %w", name, err)
	}
	source, err := backup.ManifestServer(o.Dir, name)
	if err != nil {
		return err
	}
	dest, err := backup.ServerID(ctx, o.AdminURL)
	if err != nil {
		return err
	}
	pool, err := database.Connect(ctx, o.AdminURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	r := jobqueue.Restored{BackupName: name, DestinationServer: dest, Resumed: resume, ResumedPaidAI: resumeAI}
	if source != "" {
		r.SourceServer = &source
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	after, err := jobqueue.HoldAfterRestore(ctx, tx, r)
	if err != nil {
		return fmt.Errorf("restored %s, but its jobs were not held; do not start the workers, run this restore again: %w", name, err)
	}
	// A copy of another server (a staging copy of production) spends no
	// one's AI credit until the agency running it says so.
	pausedAI := r.FromAnotherServer() && !resumeAI
	if pausedAI {
		if err := aipaid.Set(ctx, tx, true, restoredPaused, "hp restore"); err != nil {
			return fmt.Errorf("restored %s, but paid AI could not be paused; do not start the workers, run this restore again: %w", name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Print(jobsSummary(r, after))
	if pausedAI {
		fmt.Println(pausedAINote)
	}
	return nil
}

// restoredPaused is why paid AI is paused after a restore from another
// server (every company sees it: no names).
const restoredPaused = "Restored from another server: paid AI stays paused until the agency that runs this server turns it back on."

// pausedAINote tells the person restoring who can turn paid AI back on: on
// a server with several agencies, only the one HP_BACKUP_TENANT names may
// in Settings, so without it only hp paid-ai can.
const pausedAINote = "paid AI: paused, as the backup is from another server. Turn it back on with `hp paid-ai on`, " +
	"or as an owner of the agency that runs this server in Settings > System (the agency HP_BACKUP_TENANT names; " +
	"without it, only when the server has a single agency)"

// jobsSummary says what the restore did with the jobs, for the person restoring.
func jobsSummary(r jobqueue.Restored, after jobqueue.AfterRestore) string {
	var b strings.Builder
	switch {
	case r.SourceServer == nil:
		b.WriteString("jobs: the backup does not say which server it was made on\n")
	case r.FromAnotherServer():
		fmt.Fprintf(&b, "jobs: restored from another server (the backup's %s, this one %s)\n", *r.SourceServer, r.DestinationServer)
	default:
		b.WriteString("jobs: restored on the server the backup was made on\n")
	}
	if r.Resumed {
		b.WriteString("jobs: -resume-outbound: emails, calendar updates, webhooks and automations waiting in the backup will run\n")
	}
	if r.ResumedPaidAI {
		b.WriteString("jobs: -resume-paid-ai: paid AI work waiting in the backup will run\n")
	}
	switch {
	case after.HeldTotal() == 0 && !(r.Resumed && r.ResumedPaidAI):
		b.WriteString("jobs: none waiting acts outside HiringPlug or asks a paid AI: nothing held\n")
	case after.HeldTotal() > 0:
		parts := make([]string, len(after.Held))
		for i, h := range after.Held {
			parts[i] = fmt.Sprintf("%s %d", h.Kind, h.Count)
		}
		fmt.Fprintf(&b, "jobs: held %d that act outside HiringPlug or ask a paid AI (%s) until someone releases or cancels them: `hp jobs list`, then `hp jobs release -yes` or `hp jobs cancel -yes` (every company), or each company in Settings > System\n",
			after.HeldTotal(), strings.Join(parts, ", "))
		if r.FromAnotherServer() {
			fmt.Fprintf(&b, "jobs: the server's alert email waits until then (%d hours at most)\n", int(jobqueue.AlertsWaitFor.Hours()))
		}
	}
	if after.Reset > 0 {
		fmt.Fprintf(&b, "jobs: %d interrupted jobs made ready to run again\n", after.Reset)
	}
	return b.String()
}
