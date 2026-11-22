package app

// The notifications module reads its list from the modules that own the
// records (jobs, pipeline, tasks, offers, interviews) and emails through
// identity and emails: these adapters are where they meet.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notifications"
)

type notificationSources struct{ m *Modules }

func (a notificationSources) OwnedJobs(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	return a.m.Jobs.OwnedBy(ctx, tx, userID)
}

func (a notificationSources) Applicants(ctx context.Context, tx pgx.Tx, jobIDs []string, since time.Time) ([]notifications.Record, error) {
	rows, err := a.m.Pipeline.ApplicantsSince(ctx, tx, jobIDs, since)
	out := make([]notifications.Record, len(rows))
	for i, r := range rows {
		out[i] = notifications.Record{ID: r.ID, ApplicationID: r.ID, At: r.At}
	}
	return out, err
}

func (a notificationSources) DueTasks(ctx context.Context, tx pgx.Tx, userID string, day time.Time) ([]notifications.Task, error) {
	rows, err := a.m.Automations.DueTasks(ctx, tx, userID, day)
	out := make([]notifications.Task, len(rows))
	for i, r := range rows {
		out[i] = notifications.Task{ID: r.ID, Title: r.Title, DueOn: r.DueOn, ApplicationID: r.ApplicationID, CreatedAt: r.CreatedAt}
	}
	return out, err
}

func (a notificationSources) OffersToApprove(ctx context.Context, tx pgx.Tx, userID string) ([]notifications.Record, error) {
	rows, err := a.m.Offers.AwaitingApprovalBy(ctx, tx, userID)
	out := make([]notifications.Record, len(rows))
	for i, r := range rows {
		out[i] = notifications.Record{ID: r.ID, ApplicationID: r.ApplicationID, At: r.At}
	}
	return out, err
}

func (a notificationSources) Interviews(ctx context.Context, tx pgx.Tx, userID string, from, to time.Time) ([]notifications.Interview, error) {
	rows, err := a.m.Interviews.UpcomingFor(ctx, tx, userID, from, to)
	out := make([]notifications.Interview, len(rows))
	for i, r := range rows {
		out[i] = notifications.Interview{ID: r.ID, ApplicationID: r.ApplicationID, Title: r.Title, StartsAt: r.StartsAt, CreatedAt: r.CreatedAt}
	}
	return out, err
}

func (a notificationSources) Applications(ctx context.Context, tx pgx.Tx, ids []string) (map[string]notifications.Application, error) {
	briefs, err := a.m.Pipeline.Briefs(ctx, tx, ids)
	out := make(map[string]notifications.Application, len(briefs))
	for id, b := range briefs {
		out[id] = notifications.Application{CandidateID: b.CandidateID, CandidateName: b.CandidateName, JobTitle: b.JobTitle, CompanyName: b.CompanyName}
	}
	return out, err
}

// notificationMailer sends the morning email to the user's own address, as
// an account notice (not a candidate or client email).
type notificationMailer struct{ m *Modules }

func (a notificationMailer) EmailUser(ctx context.Context, tx pgx.Tx, tenantID, userID, subject, body string) (bool, error) {
	contacts, err := a.m.Identity.ActiveContacts(ctx, tx, []string{userID})
	c, ok := contacts[userID]
	if err != nil || !ok {
		return false, err
	}
	return true, a.m.Emails.Notify(ctx, tx, tenantID, emails.Notice{To: []string{c.Email}, Subject: subject, Body: body, AccountNotice: true})
}
