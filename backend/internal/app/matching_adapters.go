package app

// Adapters giving the matching module what it needs from other modules.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents"
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

func (a matchingApplications) OpenOfCandidate(ctx context.Context, tx pgx.Tx, candidateID string) ([]matching.Application, error) {
	apps, err := a.pipeline.OpenOfCandidate(ctx, tx, candidateID)
	out := make([]matching.Application, len(apps))
	for i, x := range apps {
		out[i] = matching.Application{ID: x.ID, CandidateID: x.CandidateID, JobID: x.JobID, Stage: x.Stage}
	}
	return out, err
}

func (a matchingApplications) CandidatesOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]string, error) {
	return a.pipeline.CandidatesOnJob(ctx, tx, jobID)
}

type matchingCandidates struct{ candidates *candidates.Module }

func (a matchingCandidates) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]matching.CandidateBrief, error) {
	briefs, err := a.candidates.Briefs(ctx, tx, ids)
	out := make(map[string]matching.CandidateBrief, len(briefs))
	for id, b := range briefs {
		out[id] = matching.CandidateBrief{Name: b.Name, Title: b.CurrentTitle}
	}
	return out, err
}

// Pool runs the user's advanced search for the AI candidate search: who to
// consider, and what the AI may read of their profile (no name, contact or salary).
func (a matchingCandidates) Pool(ctx context.Context, tx pgx.Tx, p authz.Principal, search api.CandidateSearch, limit int) ([]matching.PoolCandidate, error) {
	page, err := a.candidates.SearchIn(ctx, tx, p, candidates.FilterOf(search, p), limit)
	if err != nil {
		return nil, err
	}
	out := make([]matching.PoolCandidate, len(page.Items))
	for i, h := range page.Items {
		out[i] = matching.PoolCandidate{ID: h.ID, Profile: poolProfile(h.Summary)}
	}
	return out, nil
}

func poolProfile(s candidates.Summary) string {
	var parts []string
	add := func(label string, v *string) {
		if v != nil && strings.TrimSpace(*v) != "" {
			parts = append(parts, label+": "+strings.TrimSpace(*v))
		}
	}
	add("Current title", s.CurrentTitle)
	add("Current company", s.CurrentCompany)
	add("Location", s.Location)
	add("Industry", s.Industry)
	add("Work authorization", s.WorkAuthorizationStatus)
	if s.NoticePeriod != nil {
		parts = append(parts, fmt.Sprintf("Notice period: %d %s", s.NoticePeriod.Value, s.NoticePeriod.Unit))
	}
	return strings.Join(parts, "; ")
}

type matchingCVs struct{ documents *documents.Module }

func (a matchingCVs) RankCVsByText(ctx context.Context, tx pgx.Tx, text string, ids []string, limit int) ([]string, error) {
	return a.documents.RankCVsByText(ctx, tx, text, ids, limit)
}

func (a matchingCVs) LatestCVText(ctx context.Context, tx pgx.Tx, candidateID string) (string, error) {
	return a.documents.LatestCVText(ctx, tx, candidateID)
}

func (a matchingCVs) SearchCVs(ctx context.Context, tx pgx.Tx, text string, exclude []string, limit int) ([]matching.CVHit, error) {
	hits, err := a.documents.SearchCVs(ctx, tx, text, exclude, limit)
	out := make([]matching.CVHit, len(hits))
	for i, h := range hits {
		out[i] = matching.CVHit{CandidateID: h.CandidateID, Rank: h.Rank, Snippet: h.Snippet}
	}
	return out, err
}

// matchingHooks scores candidates when they are put on a job or a new CV of
// theirs is read (pipeline and documents call them; matching scores).
func matchingHooks(m *Modules) {
	m.Pipeline.WhenAdded(func(ctx context.Context, tx pgx.Tx, tenantID, jobID string, a pipeline.OnJob) error {
		return m.Matching.WhenAdded(ctx, tx, tenantID, matching.Application{ID: a.ID, CandidateID: a.CandidateID, JobID: jobID, Stage: a.Stage})
	})
	m.Documents.WhenTextRead(m.Matching.WhenCVRead)
}

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
