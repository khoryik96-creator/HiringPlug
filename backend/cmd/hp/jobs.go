package main

// On the server, for every company at once (the database owner's
// connection, HP_DATABASE_ADMIN_URL), so a restore never waits on a company
// nobody looks after, and on a server whose agencies all run their own
// companies (no HP_BACKUP_TENANT):
//
//	hp jobs list                          the jobs a restore held, every company's
//	hp jobs release -yes [job_...]        let them run (all of them, or one)
//	hp jobs cancel [-yes] [job_...]       cancel them: never run (all needs -yes)
//	hp paid-ai on|off [-reason TEXT]      turn the server's paid AI back on, or pause it
//
// Each change is in the audit log of the company it concerns (paid AI: the
// agency that runs the server, or every company), by "system".

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/aipaid"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// heldListed is how many held jobs hp jobs reads at most.
const heldListed = 100000

func runJobs(ctx context.Context, args []string) error {
	c := command{name: "jobs", synopsis: "list | release -yes [job_...] | cancel [-yes] [job_...]", min: 1, max: 2}
	fs := c.flags()
	yes := fs.Bool("yes", false, "release (it may email, post or bill again: what was held may have happened before the backup was put back), or cancel every held job")
	names, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	action, jobID := names[0], ""
	if len(names) == 2 {
		jobID = names[1]
	}
	switch {
	case action == "list" && jobID != "":
		return errors.New("hp jobs list takes no job; usage: hp jobs " + c.synopsis)
	case action != "list" && action != "release" && action != "cancel":
		return fmt.Errorf("hp jobs: unknown action %q; usage: hp jobs %s", action, c.synopsis)
	case action == "release" && !*yes:
		return errors.New("releasing held jobs may send emails, post webhooks or update calendars a second time (they may have happened before the backup was put back): check them with hp jobs list, then run hp jobs release -yes" + withJob(jobID))
	case action == "cancel" && jobID == "" && !*yes:
		return errors.New("cancelling every held job means none of them ever runs: check them with hp jobs list, then run hp jobs cancel -yes")
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	held, err := jobqueue.Held(ctx, admin, jobqueue.Everywhere, heldListed)
	if err != nil {
		return err
	}
	if action == "list" {
		return listHeld(held)
	}
	if jobID != "" {
		held = slices.DeleteFunc(held, func(j jobqueue.HeldJob) bool { return j.ID != jobID })
		if len(held) == 0 {
			return fmt.Errorf("no held job %s (hp jobs list shows them)", jobID)
		}
	}
	total := 0
	for _, tenant := range companiesOf(held) {
		n, err := settleCompany(ctx, admin, tenant, jobID, action == "release")
		if err != nil {
			return err
		}
		total += n
	}
	verb := "cancelled (never run)"
	if action == "release" {
		verb = "released"
	}
	fmt.Printf("%d held jobs %s\n", total, verb)
	return nil
}

func withJob(id string) string {
	if id == "" {
		return ""
	}
	return " " + id
}

func listHeld(held []jobqueue.HeldJob) error {
	if len(held) == 0 {
		fmt.Println("no held jobs")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "JOB\tKIND\tCOMPANY\tQUEUED\tDUE")
	for _, j := range held {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s (%s)\t%s\t%s\n", j.ID, j.Kind, j.Company, j.TenantID,
			j.CreatedAt.UTC().Format("2006-01-02 15:04"), j.RunAfter.UTC().Format("2006-01-02 15:04"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("%d held: hp jobs release -yes, or hp jobs cancel -yes (one: add its job id)\n", len(held))
	return nil
}

// companiesOf is the companies of held, each once.
func companiesOf(held []jobqueue.HeldJob) []string {
	var out []string
	for _, j := range held {
		if !slices.Contains(out, j.TenantID) {
			out = append(out, j.TenantID)
		}
	}
	return out
}

// settleCompany releases or cancels a company's held jobs (or one of
// them), audited in its own log.
func settleCompany(ctx context.Context, admin *pgxpool.Pool, tenant, jobID string, release bool) (int, error) {
	var n int
	err := database.InTx(ctx, admin, authz.System(tenant), func(tx pgx.Tx) error {
		settle, one, all := jobqueue.CancelHeld, "settings.job_cancelled", "settings.held_jobs_cancelled"
		if release {
			settle, one, all = jobqueue.Release, "settings.job_released", "settings.held_jobs_released"
		}
		var err error
		if n, err = settle(ctx, tx, tenant, jobID); err != nil || n == 0 {
			return err
		}
		entry := audit.Entry{Action: all, EntityType: "tenant", EntityID: tenant, Changes: map[string]any{"jobs": n, "by": "hp jobs"}}
		if jobID != "" {
			entry = audit.Entry{Action: one, EntityType: "job", EntityID: jobID, Changes: map[string]any{"by": "hp jobs"}}
		}
		return audit.Record(ctx, tx, entry)
	})
	return n, err
}

func runPaidAI(ctx context.Context, args []string) error {
	c := command{name: "paid-ai", synopsis: "on | off [-reason TEXT]", min: 1, max: 1}
	fs := c.flags()
	reason := fs.String("reason", "", "why it is paused (shown in Settings > System to every company)")
	names, err := c.parse(fs, args)
	if err != nil {
		return err
	}
	var paused bool
	switch names[0] {
	case "on":
		if *reason != "" {
			return errors.New("-reason goes with hp paid-ai off (why it is paused)")
		}
	case "off":
		paused = true
		if *reason = strings.TrimSpace(*reason); *reason == "" {
			*reason = "Paused on the server."
		}
	default:
		return fmt.Errorf("hp paid-ai takes on or off, got %q", names[0])
	}
	admin, err := adminPool(ctx)
	if err != nil {
		return err
	}
	defer admin.Close()
	// As the database's owner: a switch whose row was lost is put back.
	if _, err := admin.Exec(ctx, `INSERT INTO ai_paid_control DEFAULT VALUES ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	if err := aipaid.Set(ctx, admin, paused, *reason, "hp paid-ai"); err != nil {
		return err
	}
	if err := auditPaidAI(ctx, admin, paused, *reason); err != nil {
		return err
	}
	if !paused {
		fmt.Println("paid AI: on (each company within its own AI choice and allowance)")
		if os.Getenv("HP_AI_PAID") == "off" {
			fmt.Println("HP_AI_PAID=off is set here: if the API and workers have it too, they keep paid AI paused whatever this says")
		}
		return nil
	}
	fmt.Println("paid AI: paused on the whole server:", *reason)
	return nil
}

// auditPaidAI records the change in the log of the agency that runs the
// server (HP_BACKUP_TENANT, or the only one), else of every company: it
// concerns them all.
func auditPaidAI(ctx context.Context, admin *pgxpool.Pool, paused bool, reason string) error {
	tenants := []string{}
	if op := os.Getenv("HP_BACKUP_TENANT"); op != "" {
		tenants = append(tenants, op)
	} else {
		rows, err := admin.Query(ctx, `SELECT id FROM tenant ORDER BY id`)
		if err != nil {
			return err
		}
		if tenants, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
	}
	action := "settings.paid_ai_resumed"
	if paused {
		action = "settings.paid_ai_paused"
	}
	for _, tenant := range tenants {
		err := database.InTx(ctx, admin, authz.System(tenant), func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "server", EntityID: "paid_ai",
				Changes: map[string]any{"paused": paused, "reason": reason, "by": "hp paid-ai"}})
		})
		if err != nil {
			return err
		}
	}
	return nil
}
