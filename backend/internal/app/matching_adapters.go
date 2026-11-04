package app

// Adapters giving the matching module what it needs from other modules.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

type matchingApplications struct{ pipeline *pipeline.Module }

func (a matchingApplications) ForMatching(ctx context.Context, tx pgx.Tx, id string) (matching.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, []string{id})
	if err != nil {
		return matching.Application{}, err
	}
	b, ok := briefs[id]
	if !ok {
		return matching.Application{}, apperr.NotFound("application")
	}
	return matching.Application{ID: b.ID, CandidateID: b.CandidateID, JobID: b.JobID, Stage: b.Stage}, nil
}

func (a matchingApplications) OpenOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]matching.Application, error) {
	apps, err := a.pipeline.OpenOnJob(ctx, tx, jobID)
	out := make([]matching.Application, len(apps))
	for i, x := range apps {
		out[i] = matching.Application{ID: x.ID, CandidateID: x.CandidateID, JobID: jobID, Stage: x.Stage}
	}
	return out, err
}

type matchingCandidates struct{ candidates *candidates.Module }

// ForMatching gives only what identifies the candidate, to take it out of
// their CV's text: never their salary.
func (a matchingCandidates) ForMatching(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (matching.Candidate, error) {
	v, err := a.candidates.ViewIn(ctx, tx, p, id)
	if err != nil {
		return matching.Candidate{}, err
	}
	pr := v.Profile
	return matching.Candidate{Name: pr.FirstName + " " + pr.LastName, Email: deref(pr.Email), Phone: deref(pr.Phone)}, nil
}

type matchingJobs struct{ jobs *jobs.Module }

func (a matchingJobs) ForMatching(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (matching.Job, error) {
	v, err := a.jobs.ViewIn(ctx, tx, p, id)
	if err != nil {
		return matching.Job{}, err
	}
	d := v.Details
	j := matching.Job{
		ID: v.ID, Title: d.Title, CompanyName: v.CompanyName, Location: d.Location, WorkType: d.WorkType,
		WorkArrangement: d.WorkArrangement, Description: d.Description,
	} // v.Salary is never passed on
	ad, err := a.jobs.BestAd(ctx, tx, id)
	if err != nil || ad == nil {
		return j, err
	}
	j.AdTitle, j.AdText = &ad.Details.Title, ad.Details.Description // the ad's salary line is left out too
	return j, nil
}

// matchingAIUse gives the assistant matching's use of the AI, for the shared
// allowance. It looks the module up when called: matching is built after the
// assistant.
type matchingAIUse struct{ m *Modules }

func (u matchingAIUse) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return u.m.Matching.TokensSince(ctx, tx, since)
}
