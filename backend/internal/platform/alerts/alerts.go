// Package alerts tells the people who look after the server when something
// needs them (HP_ALERT_EMAIL): a backup failed or is overdue, its copy off
// the server failed, the backup service stopped, background work keeps
// failing or waits too long, or the disk is nearly full. The worker checks
// every 15 minutes and emails when a problem starts, once a day while it
// lasts, and once when it is over. (A server that is down cannot say so: an
// outside monitor calls /health/ready for that; see the server guide.)
package alerts

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Every is how often the server is checked.
const Every = 15 * time.Minute

// Problem is one thing that needs someone.
type Problem struct {
	Key  string // the same problem keeps its key from one check to the next
	Text string
}

// DB is what the checks read.
type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Checks look at the server.
type Checks struct {
	DB       DB
	FilesDir string                                            // the uploaded files: their disk must not fill up
	Disk     func(path string) (free, total uint64, err error) // nil: the real disk
	Now      func() time.Time
}

// Problems returns what needs someone now.
func (c Checks) Problems(ctx context.Context) ([]Problem, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	var out []Problem
	control, err := backup.ReadControl(ctx, c.DB)
	if err != nil {
		return nil, err
	}
	last, ok, err := backup.LastRun(ctx, c.DB)
	if err != nil {
		return nil, err
	}
	switch {
	case ok && last.Status == "failed":
		out = append(out, Problem{"backup-failed", fmt.Sprintf("The last backup failed (%s): %s", last.StartedAt.Format(time.RFC1123), deref(last.Error))})
	case ok && last.Status == "running" && now().Sub(last.StartedAt) > 6*time.Hour:
		// Hung (pg_dump waiting on a lock, say): the service still checks in.
		out = append(out, Problem{"backup-stuck", fmt.Sprintf("A backup started at %s has not finished: no backups are being made.", last.StartedAt.Format(time.RFC1123))})
	case ok && control.Auto && last.Status == "done" && now().Sub(last.StartedAt) > 50*time.Hour:
		out = append(out, Problem{"backup-old", fmt.Sprintf("No backup since %s, though the nightly backup is on.", last.StartedAt.Format(time.RFC1123))})
	}
	if ok && last.OffsiteStatus != nil && *last.OffsiteStatus == "failed" {
		out = append(out, Problem{"offsite-failed", "The last backup was not copied off the server: " + deref(last.OffsiteError)})
	}
	// Only once it has been seen: a laptop without the backup service is not a problem.
	if control.Auto && control.ServiceSeenAt != nil && now().Sub(*control.ServiceSeenAt) > 15*time.Minute {
		out = append(out, Problem{"backup-service", fmt.Sprintf("The backup service has not checked in since %s: no backups are being made.", control.ServiceSeenAt.Format(time.RFC1123))})
	}
	var dead, stuck int
	if err := c.DB.QueryRow(ctx, `SELECT dead, stuck FROM job_health(interval '1 day', interval '1 hour')`).Scan(&dead, &stuck); err != nil {
		return nil, err
	}
	if dead > 0 {
		out = append(out, Problem{"jobs-failed", fmt.Sprintf("%d background task(s) failed for good in the last day (emails, CV reading…): see Settings > System.", dead)})
	}
	if stuck > 0 {
		out = append(out, Problem{"jobs-stuck", fmt.Sprintf("%d background task(s) have waited over an hour: a worker may have stopped.", stuck)})
	}
	if c.FilesDir != "" {
		disk := c.Disk
		if disk == nil {
			disk = diskSpace
		}
		if free, total, err := disk(c.FilesDir); err == nil && total > 0 && (free < total/10 || free < 2<<30) {
			out = append(out, Problem{"disk-full", fmt.Sprintf("The disk is nearly full: %.1f GB free of %.1f GB.", gb(free), gb(total))})
		}
	}
	return out, nil
}

func gb(n uint64) float64 { return float64(n) / (1 << 30) }

func deref(s *string) string {
	if s == nil {
		return "no reason given"
	}
	return *s
}

// Watcher emails problems as they start, once a day while they last, and
// once when they are over.
type Watcher struct {
	Checks Checks
	Mail   mail.Sender
	To     []string // who looks after the server; none: problems are only logged
	Server string   // which server, e.g. https://app.youragency.com
	Log    *slog.Logger
	open   map[string]time.Time // problem key -> when it was last emailed
	text   map[string]string
}

// Run checks the server every Every until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	for {
		if err := w.Check(ctx); err != nil && ctx.Err() == nil {
			w.Log.Warn("server check failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(Every):
		}
	}
}

// Check looks once and emails what is new, still there after a day, or over.
func (w *Watcher) Check(ctx context.Context) error {
	if w.open == nil {
		w.open, w.text = map[string]time.Time{}, map[string]string{}
	}
	now := time.Now
	if w.Checks.Now != nil {
		now = w.Checks.Now
	}
	problems, err := w.Checks.Problems(ctx)
	if err != nil {
		return err
	}
	var due []Problem
	seen := map[string]bool{}
	for _, p := range problems {
		seen[p.Key] = true
		if sent, ok := w.open[p.Key]; !ok || now().Sub(sent) >= 24*time.Hour {
			due = append(due, p)
		}
	}
	var over []string
	for key := range w.open {
		if !seen[key] {
			over = append(over, w.text[key])
		}
	}
	slices.Sort(over)
	if len(due) == 0 && len(over) == 0 {
		return nil
	}
	for _, p := range due {
		w.Log.Warn("server problem", "problem", p.Key, "text", p.Text)
	}
	// Restored, with jobs held until someone releases them: a copy of
	// another server (staging) must not email its people either. The
	// problems are emailed once the restore is settled.
	if r, ok, err := jobqueue.LastRestore(ctx, w.Checks.DB); err != nil {
		return err
	} else if ok && r.Holding() {
		w.Log.Info("alert email waits: the server was restored and its held jobs are not released yet (Settings > System)")
		return nil
	}
	if len(w.To) > 0 {
		if err := w.Mail.Send(ctx, w.message(problems, due, over)); err != nil {
			return fmt.Errorf("emailing the alert: %w", err) // tried again at the next check
		}
	}
	for _, p := range due {
		w.open[p.Key], w.text[p.Key] = now(), p.Text
	}
	for key := range w.open {
		if !seen[key] {
			delete(w.open, key)
			delete(w.text, key)
		}
	}
	return nil
}

func (w *Watcher) message(all, due []Problem, over []string) mail.Message {
	var b strings.Builder
	subject := "HiringPlug: all clear"
	if len(all) > 0 { // some fixed, some still there
		subject = fmt.Sprintf("HiringPlug: %d fixed, %d still to do on the server", len(over), len(all))
	}
	if len(due) > 0 {
		subject = fmt.Sprintf("HiringPlug needs you: %d problem(s) on the server", len(all))
		b.WriteString("The server needs someone:\n\n")
		for _, p := range all {
			fmt.Fprintf(&b, "- %s\n", p.Text)
		}
	}
	if len(over) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Over now:\n\n")
		for _, t := range over {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}
	fmt.Fprintf(&b, "\nServer: %s (Settings > System shows more).\n", w.Server)
	return mail.Message{FromName: "HiringPlug server", To: w.To, Subject: subject, Body: b.String()}
}
