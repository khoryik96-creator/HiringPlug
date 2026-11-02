package app

// Adapters giving the emails module what it needs from other modules, in its
// own types.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
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

type emailInterviews struct{ interviews *interviews.Module }

func (a emailInterviews) Brief(ctx context.Context, tx pgx.Tx, id string) (emails.Interview, error) {
	b, err := a.interviews.Brief(ctx, tx, id)
	d := b.Details
	return emails.Interview{
		ApplicationID: b.ApplicationID, Title: d.Title, StartsAt: d.StartsAt, Minutes: d.Duration,
		Mode: d.Mode, Location: d.Location, Instructions: d.Instructions,
	}, err
}
