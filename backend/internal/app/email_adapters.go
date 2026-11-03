package app

// Adapters giving the emails module what it needs from other modules, in its
// own types.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
)

type emailCandidates struct{ candidates *candidates.Module }

func (a emailCandidates) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]emails.Candidate, error) {
	briefs, err := a.candidates.Briefs(ctx, tx, ids)
	out := make(map[string]emails.Candidate, len(briefs))
	for id, b := range briefs {
		out[id] = emails.Candidate{FullName: b.Name, FirstName: b.FirstName, Email: b.Email}
	}
	return out, err
}

// emailApplications combines an application (pipeline) with its job's
// hiring contact (jobs, contacts).
type emailApplications struct {
	pipeline *pipeline.Module
	jobs     *jobs.Module
	contacts *contacts.Module
}

func (a emailApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]emails.Application, error) {
	apps, err := a.pipeline.Briefs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	jobIDs := make([]string, 0, len(apps))
	for _, b := range apps {
		jobIDs = append(jobIDs, b.JobID)
	}
	jobBriefs, err := a.jobs.Briefs(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	var contactIDs []string
	for _, j := range jobBriefs {
		if j.ContactID != nil {
			contactIDs = append(contactIDs, *j.ContactID)
		}
	}
	contactBriefs, err := a.contacts.Briefs(ctx, tx, contactIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]emails.Application, len(apps))
	for id, b := range apps {
		app := emails.Application{CandidateID: b.CandidateID, JobTitle: b.JobTitle, CompanyName: b.CompanyName}
		if j := jobBriefs[b.JobID]; j.ContactID != nil {
			if c, ok := contactBriefs[*j.ContactID]; ok {
				app.ClientEmail, app.ClientContact = c.Email, c.Name
			}
		}
		out[id] = app
	}
	return out, nil
}

// emailInterviews reads the interviews module when called: the two modules
// use each other (drafts read interviews; reminders send email), so it is
// built after this one.
type emailInterviews struct{ modules *Modules }

func (a emailInterviews) Brief(ctx context.Context, tx pgx.Tx, id string) (emails.Interview, error) {
	b, err := a.modules.Interviews.Brief(ctx, tx, id)
	d := b.Details
	return emails.Interview{
		ApplicationID: b.ApplicationID, Title: d.Title, StartsAt: d.StartsAt, Minutes: d.Duration,
		Mode: d.Mode, Location: d.Location, Instructions: d.Instructions,
	}, err
}

// interviewMailer emails interviewers (reminders): their addresses come from
// identity, the email from the emails module.
type interviewMailer struct {
	identity *identity.Module
	emails   *emails.Module
}

func (a interviewMailer) EmailUsers(ctx context.Context, tx pgx.Tx, tenantID string, userIDs []string, m interviews.Message) (int, error) {
	contacts, err := a.identity.ActiveContacts(ctx, tx, userIDs)
	if err != nil || len(contacts) == 0 {
		return 0, err
	}
	to := make([]string, 0, len(contacts))
	for _, id := range userIDs {
		if c, ok := contacts[id]; ok {
			to = append(to, c.Email)
		}
	}
	return len(to), a.emails.Notify(ctx, tx, tenantID, emails.Notice{To: to, Subject: m.Subject, Body: m.Body, ApplicationID: &m.ApplicationID})
}

// identityMailer sends identity's invitations and password resets through the
// emails module (built after identity, so read when called).
type identityMailer struct{ modules *Modules }

func (a identityMailer) Notify(ctx context.Context, tx pgx.Tx, tenantID string, to []string, subject, body string) error {
	return a.modules.Emails.Notify(ctx, tx, tenantID, emails.Notice{To: to, Subject: subject, Body: body})
}
