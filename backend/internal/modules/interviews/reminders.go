package interviews

// Reminders: the interviewers are emailed the day before an interview. The
// reminder is queued when the interview is scheduled (and again when it is
// moved, or scheduled again after a cancellation); when it runs it checks
// the interview is still at that time and still on, so a moved or cancelled
// interview is never reminded of at its old time.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/humantime"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// RemindBefore is how long before an interview its interviewers are reminded.
const RemindBefore = 24 * time.Hour

// ReminderTime returns when to remind of an interview starting at startsAt:
// a day before, or straight away when that has passed, but not at all for an
// interview starting within the hour (a reminder would come too late).
func ReminderTime(startsAt, now time.Time) (time.Time, bool) {
	if !startsAt.After(now.Add(time.Hour)) {
		return time.Time{}, false
	}
	at := startsAt.Add(-RemindBefore)
	if at.Before(now) {
		at = now
	}
	return at, true
}

var modeLabel = map[string]string{"in_person": "In person", "video": "Video call", "phone": "Phone call"}

// ReminderText writes the reminder email.
func ReminderText(d Details, app Application, interviewers []string, loc *time.Location, publicURL string) (subject, body string) {
	when := humantime.When(d.StartsAt, loc)
	subject = "Reminder: " + d.Title + " with " + app.CandidateName + ", " + when
	var b strings.Builder
	b.WriteString("You are interviewing " + app.CandidateName + " for " + app.JobTitle + ".\n\n")
	b.WriteString("When: " + when + " (" + humantime.Length(d.Duration) + ")\n")
	b.WriteString("How: " + modeLabel[d.Mode] + "\n")
	if d.Location != nil && *d.Location != "" {
		b.WriteString("Where: " + *d.Location + "\n")
	}
	b.WriteString("Interviewers: " + strings.Join(interviewers, ", ") + "\n\n")
	b.WriteString("The candidate's profile and CV, and your feedback after the interview:\n")
	b.WriteString(publicURL + "/#/candidates/" + app.CandidateID + "\n\nHiringPlug")
	return subject, b.String()
}

// queueReminder queues the reminder for an interview at its current time.
func (s *Service) queueReminder(ctx context.Context, tx pgx.Tx, tenantID, id string, d Details) error {
	if d.Status != "scheduled" {
		return nil
	}
	at, ok := ReminderTime(d.StartsAt, time.Now())
	if !ok {
		return nil
	}
	_, err := jobqueue.EnqueueAt(ctx, tx, tenantID, api.JobKindInterviewRemind,
		api.InterviewRemindJob{InterviewId: id, StartsAt: d.StartsAt}, at)
	return err
}

// remind handles interview.remind: it emails the interviewers, once per
// interview time, if the interview is still on at the time it was queued for.
func (s *Service) remind(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.InterviewRemindJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.GetInterview(ctx, payload.InterviewId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	d := detailsOf(row)
	reminded := row.RemindedFor != nil && row.RemindedFor.Equal(row.StartsAt)
	if d.Status != "scheduled" || !row.StartsAt.Equal(payload.StartsAt) || reminded || !row.StartsAt.After(time.Now()) {
		return nil // cancelled, moved (the new time has its own reminder), already reminded, or over
	}
	apps, err := s.applications.Briefs(ctx, tx, []string{row.ApplicationID})
	if err != nil {
		return err
	}
	app, ok := apps[row.ApplicationID]
	if !ok {
		return nil
	}
	names, err := s.users.UserNames(ctx, tx, d.Interviewers)
	if err != nil {
		return err
	}
	list := make([]string, 0, len(d.Interviewers))
	for _, id := range d.Interviewers {
		if n := names[id]; n != "" {
			list = append(list, n)
		}
	}
	subject, body := ReminderText(d, app, list, s.settings.TimeZone, s.settings.PublicURL)
	sent, err := s.mailer.EmailUsers(ctx, tx, job.TenantID, d.Interviewers, Message{Subject: subject, Body: body, ApplicationID: row.ApplicationID})
	if err != nil {
		return err
	}
	if err := q.MarkReminded(ctx, store.MarkRemindedParams{ID: row.ID, StartsAt: &row.StartsAt}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "interview.reminded", EntityType: "interview", EntityID: row.ID,
		Changes: map[string]any{"startsAt": row.StartsAt, "emailed": sent},
	})
}
