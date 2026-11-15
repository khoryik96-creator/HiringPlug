package automations

// Events (from the audit log) schedule runs; a run, when due, checks it still
// applies and then adds its tasks and sends its emails (or puts them up for
// approval). Approvals send from the approver's mailbox.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// Subject types: what a run is about.
const (
	OnApplication = "application"
	OnInterview   = "interview"
	OnPlacement   = "placement"
)

// event is what happened, as automations see it: what it is about and, per
// trigger, the moment (or date) it counts from.
type event struct {
	subjectType, subjectID, applicationID string
	stage                                 string // the stage moved to
	anchors                               map[string]time.Time
}

// onEvent handles automation.event: it schedules a run for each switched-on
// automation the change starts.
func (s *Service) onEvent(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var j api.AutomationEventJob
	if err := job.Decode(&j); err != nil {
		return err
	}
	q := store.New(tx)
	rows, err := q.EnabledAutomations(ctx)
	if err != nil || len(rows) == 0 {
		return err
	}
	e, ok, err := s.eventOf(ctx, tx, j)
	if err != nil || !ok {
		return err
	}
	apps, err := s.deps.Applications.Applications(ctx, tx, []string{e.applicationID})
	if err != nil {
		return err
	}
	app, ok := apps[e.applicationID]
	if !ok {
		return nil
	}
	now := s.now()
	for _, row := range rows {
		r, err := ruleOf(row)
		if err != nil {
			return err
		}
		anchor, ok := e.anchors[r.Trigger]
		if !ok || (r.Trigger == StageChanged && *r.Stage != e.stage) || (r.CompanyID != nil && *r.CompanyID != app.CompanyID) {
			continue
		}
		about := DueAt(Rule{Trigger: r.Trigger}, anchor, s.deps.TimeZone)
		when, ok := When(DueAt(r, anchor, s.deps.TimeZone), about, now)
		if !ok { // too late now: a run waiting for the old date must not go then
			late := "The date moved: it would come too late."
			if _, err := q.SkipScheduledRun(ctx, store.SkipScheduledRunParams{AutomationID: row.ID, SubjectID: e.subjectID, Detail: &late}); err != nil {
				return err
			}
			continue
		}
		run, err := q.ScheduleRun(ctx, store.ScheduleRunParams{
			AutomationID: row.ID, SubjectType: e.subjectType, SubjectID: e.subjectID, ApplicationID: e.applicationID, DueAt: when,
		})
		if database.IsNoRows(err) {
			continue // already ran, or already due then
		}
		if err != nil {
			return err
		}
		if _, err := jobqueue.EnqueueAt(ctx, tx, job.TenantID, api.JobKindAutomationRun, api.AutomationRunJob{RunId: run.ID, DueAt: run.DueAt}, run.DueAt); err != nil {
			return err
		}
	}
	return nil
}

// eventOf reads what an audit entry is about; false when no automation
// starts from it (a cancelled interview, a placement that fell through, ...).
func (s *Service) eventOf(ctx context.Context, tx pgx.Tx, j api.AutomationEventJob) (event, bool, error) {
	now := s.now()
	switch j.Action {
	case "application.created":
		return event{subjectType: OnApplication, subjectID: j.EntityId, applicationID: j.EntityId,
			anchors: map[string]time.Time{ApplicationReceived: now}}, true, nil
	case "application.moved":
		to := ""
		if j.Changes != nil {
			if c, ok := (*j.Changes)["stage"].(map[string]any); ok {
				to, _ = c["to"].(string)
			}
		}
		return event{subjectType: OnApplication, subjectID: j.EntityId, applicationID: j.EntityId, stage: to,
			anchors: map[string]time.Time{StageChanged: now}}, to != "", nil
	case "interview.scheduled", "interview.updated":
		iv, ok, err := s.deps.Interviews.Interview(ctx, tx, j.EntityId)
		if err != nil || !ok || iv.Status != "scheduled" {
			return event{}, false, err
		}
		return event{subjectType: OnInterview, subjectID: j.EntityId, applicationID: iv.ApplicationID,
			anchors: map[string]time.Time{InterviewScheduled: iv.StartsAt}}, true, nil
	case "placement.recorded", "placement.updated", "placement.invoicing_set":
		pl, ok, err := s.deps.Placements.Placement(ctx, tx, j.EntityId)
		if err != nil || !ok || pl.Status != "active" {
			return event{}, false, err
		}
		anchors := map[string]time.Time{PlacementStarts: pl.StartDate, GuaranteeEnds: pl.GuaranteeEnds}
		if !pl.Paid {
			anchors[InvoiceDue] = pl.DueOn
		}
		return event{subjectType: OnPlacement, subjectID: j.EntityId, applicationID: pl.ApplicationID, anchors: anchors}, true, nil
	}
	return event{}, false, nil
}

// facts is what a due run is about, read again.
type facts struct {
	app         Application
	interviewID *string
	recruiterID *string
}

// recheck reads what a run is about again; reason says why it no longer
// applies ("" when it does).
func (s *Service) recheck(ctx context.Context, tx pgx.Tx, run store.AutomationRun, r Rule) (facts, string, error) {
	apps, err := s.deps.Applications.Applications(ctx, tx, []string{run.ApplicationID})
	if err != nil {
		return facts{}, "", err
	}
	app, ok := apps[run.ApplicationID]
	if !ok {
		return facts{}, "The application is gone.", nil
	}
	f := facts{app: app, recruiterID: app.RecruiterID}
	switch run.SubjectType {
	case OnApplication:
		if r.Trigger == StageChanged && app.Stage != *r.Stage {
			return f, "The candidate is no longer at " + *r.Stage + ".", nil
		}
	case OnInterview:
		iv, ok, err := s.deps.Interviews.Interview(ctx, tx, run.SubjectID)
		if err != nil {
			return f, "", err
		}
		if !ok || iv.Status != "scheduled" {
			return f, "The interview is no longer scheduled.", nil
		}
		f.interviewID = &run.SubjectID
	case OnPlacement:
		pl, ok, err := s.deps.Placements.Placement(ctx, tx, run.SubjectID)
		if err != nil {
			return f, "", err
		}
		switch {
		case !ok || pl.Status != "active":
			return f, "The placement no longer stands.", nil
		case r.Trigger == InvoiceDue && pl.Paid:
			return f, "The invoice is paid.", nil
		}
		if pl.RecruiterID != nil {
			f.recruiterID = pl.RecruiterID
		}
	}
	if r.CompanyID != nil && *r.CompanyID != app.CompanyID {
		return f, "The job is no longer at that client.", nil
	}
	return f, "", nil
}

// run handles automation.run: a run is due.
func (s *Service) run(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var j api.AutomationRunJob
	if err := job.Decode(&j); err != nil {
		return err
	}
	q := store.New(tx)
	run, err := q.LockRun(ctx, j.RunId)
	if database.IsNoRows(err) {
		return nil // its automation was removed
	}
	if err != nil {
		return err
	}
	if run.Status != Scheduled || !run.DueAt.Equal(j.DueAt) {
		return nil // already run, or moved to another time (another job)
	}
	row, err := q.GetAutomation(ctx, run.AutomationID)
	if err != nil {
		return err
	}
	r, err := ruleOf(row)
	if err != nil {
		return err
	}
	finish := func(status, detail string) error {
		_, err := q.FinishRun(ctx, store.FinishRunParams{ID: run.ID, Status: status, Detail: &detail})
		return err
	}
	if !r.Enabled {
		return finish(Skipped, "The automation was switched off.")
	}
	f, reason, err := s.recheck(ctx, tx, run, r)
	if err != nil {
		return err
	}
	if reason != "" {
		return finish(Skipped, reason)
	}
	recruiter := f.recruiterID
	if recruiter == nil {
		recruiter = &row.CreatedBy
	}
	p, ok, err := s.deps.Users.PrincipalFor(ctx, tx, *recruiter)
	if err != nil {
		return err
	}
	if !ok {
		return finish(Failed, "The job has no active recruiter to send it from or give the task to.")
	}
	did := []string{}
	for _, a := range r.Actions {
		if a.Kind != Task {
			continue
		}
		values := map[string]string{"candidate.fullName": f.app.CandidateName, "candidate.firstName": firstName(f.app.CandidateName),
			"job.title": f.app.JobTitle, "company.name": f.app.CompanyName}
		if _, err := q.InsertTask(ctx, store.InsertTaskParams{
			UserID: p.UserID, Title: clip(FillTitle(a.Title, values), 200), DueOn: s.localToday(), ApplicationID: &run.ApplicationID,
			RunID: &run.ID, AutomationName: &row.Name,
		}); err != nil {
			return err
		}
		did = append(did, "added a task for "+p.Name)
	}
	if r.HasEmail() {
		if r.SendMode == AskFirst {
			return finish(ToApprove, sentence(append(did, "the email is waiting for approval")))
		}
		problem, err := s.sendEmails(ctx, tx, p, run, f, r)
		if err != nil {
			return err
		}
		if problem != nil && problem.Code == api.ErrorCodeEMAILUNDELIVERABLE {
			// The client contact left, or their mailbox is gone: not sent; the
			// recruiter is asked to find who to tell instead.
			if _, err := q.InsertTask(ctx, store.InsertTaskParams{
				UserID: p.UserID, Title: clip("Not emailed: "+problemText(problem)+". Update "+f.app.CompanyName+"'s contact for "+f.app.JobTitle, 200),
				DueOn: s.localToday(), ApplicationID: &run.ApplicationID, RunID: &run.ID, AutomationName: &row.Name,
			}); err != nil {
				return err
			}
			return finish(Skipped, sentence(append(did, "not emailed: "+problemText(problem), "added a task for "+p.Name)))
		}
		if problem != nil {
			return finish(Failed, sentence(append(did, "not emailed: "+problemText(problem))))
		}
		did = append(did, "emailed from "+p.Name+"'s mailbox")
	}
	logx.From(ctx).Info("automation ran", "run_id", run.ID, "automation_id", row.ID)
	return finish(Done, sentence(did))
}

// sendEmails sends a run's emails as sender, in a savepoint: problem says why
// they could not go (nothing is sent then).
func (s *Service) sendEmails(ctx context.Context, tx pgx.Tx, sender authz.Principal, run store.AutomationRun, f facts, r Rule) (*apperr.Error, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sp.Rollback(ctx) }()
	to := map[string]string{EmailCandidate: "candidate", EmailClient: "client", EmailRecruiter: "sender"}
	for _, a := range r.Actions {
		if !a.IsEmail() {
			continue
		}
		err := s.deps.Mailer.SendAutomated(ctx, sp, sender, Email{
			CandidateID: f.app.CandidateID, ApplicationID: run.ApplicationID, InterviewID: f.interviewID,
			To: to[a.Kind], Subject: a.Subject, Body: a.Body, TimeZone: s.deps.TimeZone,
		})
		if e, ok := apperr.As(err); ok {
			return e, nil
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, sp.Commit(ctx)
}

// problemText is why an email cannot go, in words (a field error's own message).
func problemText(e *apperr.Error) string {
	if fields, ok := e.Details["fields"].(apperr.FieldErrors); ok && len(fields) > 0 {
		parts := make([]string, len(fields))
		for i, f := range fields {
			parts[i] = f.Field + " " + f.Message
		}
		return strings.Join(parts, "; ")
	}
	return e.Message
}

// RunView is a run as shown in the log.
type RunView struct {
	store.ListRunsRow
	App     *Application
	Actions []Action
}

// Runs returns the log: newest first (those still due, soonest first).
func (s *Service) Runs(ctx context.Context, p authz.Principal, status *string) ([]RunView, error) {
	var out []RunView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListRuns(ctx, status)
		if err != nil {
			return err
		}
		out, err = s.runViews(ctx, tx, rows)
		return err
	})
	return out, err
}

// Approve sends the emails of a run waiting for approval, from p's mailbox.
// A run that no longer applies is skipped instead (its detail says why).
func (s *Service) Approve(ctx context.Context, p authz.Principal, id string) (RunView, error) {
	return s.decide(ctx, p, id, func(tx pgx.Tx, q *store.Queries, run store.AutomationRun, row store.Automation, r Rule) (string, string, error) {
		f, reason, err := s.recheck(ctx, tx, run, r)
		if err != nil || reason != "" {
			return Skipped, reason, err
		}
		problem, err := s.sendEmails(ctx, tx, p, run, f, r)
		if err != nil {
			return "", "", err
		}
		if problem != nil {
			return "", "", apperr.New(api.ErrorCodeAUTOMATIONNOTALLOWED, problemText(problem))
		}
		return Done, "Sent by " + p.Name + ".", nil
	})
}

// Skip leaves the emails of a run waiting for approval unsent.
func (s *Service) Skip(ctx context.Context, p authz.Principal, id string) (RunView, error) {
	return s.decide(ctx, p, id, func(pgx.Tx, *store.Queries, store.AutomationRun, store.Automation, Rule) (string, string, error) {
		return Skipped, "Skipped by " + p.Name + ".", nil
	})
}

type decision func(tx pgx.Tx, q *store.Queries, run store.AutomationRun, row store.Automation, r Rule) (status, detail string, err error)

func (s *Service) decide(ctx context.Context, p authz.Principal, id string, d decision) (RunView, error) {
	var out RunView
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		run, err := q.LockRun(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("automation run")
		}
		if err != nil {
			return err
		}
		if visible, err := s.deps.Applications.Applications(ctx, tx, []string{run.ApplicationID}); err != nil {
			return err
		} else if _, ok := visible[run.ApplicationID]; !ok {
			return apperr.NotFound("automation run") // about a candidate p does not see
		}
		if run.Status != ToApprove {
			return apperr.New(api.ErrorCodeAUTOMATIONNOTALLOWED, "This is no longer waiting for approval.")
		}
		row, err := q.GetAutomation(ctx, run.AutomationID)
		if err != nil {
			return err
		}
		r, err := ruleOf(row)
		if err != nil {
			return err
		}
		status, detail, err := d(tx, q, run, row, r)
		if err != nil {
			return err
		}
		done, err := q.FinishRun(ctx, store.FinishRunParams{ID: id, Status: status, Detail: &detail, FinishedBy: &p.UserID})
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "automation.run_" + status, EntityType: "automation_run", EntityID: id,
			Changes: map[string]any{"automationId": row.ID, "applicationId": run.ApplicationID}}); err != nil {
			return err
		}
		views, err := s.runViews(ctx, tx, []store.ListRunsRow{{
			ID: done.ID, TenantID: done.TenantID, AutomationID: done.AutomationID, SubjectType: done.SubjectType, SubjectID: done.SubjectID,
			ApplicationID: done.ApplicationID, DueAt: done.DueAt, Status: done.Status, Detail: done.Detail, CreatedAt: done.CreatedAt,
			FinishedAt: done.FinishedAt, FinishedBy: done.FinishedBy, AutomationName: row.Name, Actions: row.Actions,
		}})
		if err != nil || len(views) == 0 {
			return err
		}
		out = views[0]
		return nil
	})
	return out, err
}

func (s *Service) runViews(ctx context.Context, tx pgx.Tx, rows []store.ListRunsRow) ([]RunView, error) {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ApplicationID
	}
	apps, err := s.deps.Applications.Applications(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]RunView, 0, len(rows))
	for _, r := range rows {
		a, ok := apps[r.ApplicationID]
		if !ok {
			continue // about a candidate the user does not see (or no longer there)
		}
		rule, err := ruleOf(store.Automation{Actions: r.Actions})
		if err != nil {
			return nil, err
		}
		out = append(out, RunView{ListRunsRow: r, Actions: rule.Actions, App: &a})
	}
	return out, nil
}

func (s *Service) localToday() time.Time {
	y, m, d := s.now().In(s.deps.TimeZone).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// sentence joins what a run did: "Added a task for Aina; emailed ...".
func sentence(parts []string) string {
	if len(parts) == 0 {
		return "Nothing to do."
	}
	s := strings.Join(parts, "; ") + "."
	return strings.ToUpper(s[:1]) + s[1:]
}

func firstName(full string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(full), " ")
	return first
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
