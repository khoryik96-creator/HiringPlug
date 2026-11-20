package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviewprep"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// interviewPrepJobs gives interview prep a job as the user may see it, with
// its newest ad's text; its salary is never passed on.
type interviewPrepJobs struct{ jobs *jobs.Module }

func (a interviewPrepJobs) Exists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	return a.jobs.Exists(ctx, tx, id)
}

func (a interviewPrepJobs) ForInterviewPrep(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (interviewprep.Job, error) {
	v, err := a.jobs.ViewIn(ctx, tx, p, id)
	if err != nil {
		return interviewprep.Job{}, err
	}
	ad, err := a.jobs.AdText(ctx, tx, id)
	if err != nil {
		return interviewprep.Job{}, err
	}
	d := v.Details
	job := interviewprep.Job{Title: d.Title, CompanyID: d.CompanyID, CompanyName: v.CompanyName, Location: d.Location,
		WorkArrangement: d.WorkArrangement, AdText: ad}
	if d.Description != nil {
		job.Description = *d.Description
	}
	return job, nil // v.Salary is never passed on
}

type interviewPrepCompanies struct{ companies *companies.Module }

func (a interviewPrepCompanies) ForInterviewPrep(ctx context.Context, tx pgx.Tx, id string) (interviewprep.Company, error) {
	c, err := a.companies.ProfileOf(ctx, tx, id)
	return interviewprep.Company{Industry: c.Industry, About: c.About}, err
}
