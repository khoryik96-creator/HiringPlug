package app

// The jobs and pipeline modules' adapters: what they need from contacts,
// candidates and jobs, in their own types, so no module imports another.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
)

// jobContacts gives the jobs module what it needs from contacts, in its own
// types, so neither module imports the other.
type jobContacts struct{ contacts *contacts.Module }

// Touch marks the job's hiring contact (and their company) as touched.
func (a jobContacts) Touch(ctx context.Context, tx pgx.Tx, id string) error {
	return a.contacts.Touch(ctx, tx, id)
}

func (a jobContacts) Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]jobs.ContactInfo, error) {
	briefs, err := a.contacts.Briefs(ctx, tx, ids)
	out := make(map[string]jobs.ContactInfo, len(briefs))
	for id, b := range briefs {
		out[id] = jobs.ContactInfo{Name: b.Name, CompanyID: b.CompanyID}
	}
	return out, err
}

// pipelineCandidates and pipelineJobs give the pipeline module what it needs,
// in its own types.
type pipelineCandidates struct{ *candidates.Module }

func (a pipelineCandidates) Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]pipeline.CandidateInfo, error) {
	briefs, err := a.Briefs(ctx, tx, ids)
	out := make(map[string]pipeline.CandidateInfo, len(briefs))
	for id, b := range briefs {
		out[id] = pipeline.CandidateInfo{Name: b.Name, Title: b.CurrentTitle, Availability: b.Availability}
	}
	return out, err
}

type pipelineJobs struct{ *jobs.Module }

func (a pipelineJobs) OpenJobs(ctx context.Context, tx pgx.Tx, limit int) ([]pipeline.OpenJob, error) {
	open, err := a.Open(ctx, tx, limit)
	out := make([]pipeline.OpenJob, len(open))
	for i, j := range open {
		out[i] = pipeline.OpenJob{ID: j.ID, Title: j.Title, CompanyID: j.CompanyID, CreatedAt: j.CreatedAt}
	}
	return out, err
}

func (a pipelineJobs) Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]pipeline.JobInfo, error) {
	briefs, err := a.Briefs(ctx, tx, ids)
	out := make(map[string]pipeline.JobInfo, len(briefs))
	for id, b := range briefs {
		out[id] = pipeline.JobInfo{Title: b.Title, CompanyID: b.CompanyID}
	}
	return out, err
}
