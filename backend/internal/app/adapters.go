package app

// Adapters between modules: each converts one module's types into the small
// interface another declares, so modules never import each other.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/careers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/offers"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
)

// careersAds gives the careers module the jobs module's published ads, in
// its own Ad type (modules do not share types).
type careersAds struct{ jobs *jobs.Module }

func (a careersAds) PublishedAds(ctx context.Context, tx pgx.Tx) ([]careers.Ad, error) {
	ads, err := a.jobs.PublishedAds(ctx, tx)
	out := make([]careers.Ad, len(ads))
	for i, ad := range ads {
		out[i] = careersAd(ad)
	}
	return out, err
}

func (a careersAds) PublishedAd(ctx context.Context, tx pgx.Tx, id string) (careers.Ad, error) {
	ad, err := a.jobs.PublishedAd(ctx, tx, id)
	return careersAd(ad), err
}

func careersAd(ad jobs.Ad) careers.Ad {
	d := ad.Details
	out := careers.Ad{
		ID: ad.ID, JobID: ad.JobID, Title: d.Title, Location: d.Location, Country: d.Country, WorkType: d.WorkType,
		WorkArrangement: d.WorkArrangement, SalaryText: d.SalaryText, ClosesOn: d.ClosesOn,
	}
	text := d.Description
	if d.PublishBlind { // the client stays unnamed in public
		out.Title, text = ad.Blind.Title, ad.Blind.Description
		out.Location, out.SalaryText = ad.Blind.Location, ad.Blind.SalaryText
	}
	if text != nil {
		out.Description = *text
	}
	if ad.PublishedAt != nil {
		out.PublishedAt = *ad.PublishedAt
	}
	return out
}

// careersCandidates adds applicants through the candidates module.
type careersCandidates struct{ candidates *candidates.Module }

func (c careersCandidates) FindOrCreateApplicant(ctx context.Context, tx pgx.Tx, a careers.Applicant) (string, bool, error) {
	return c.candidates.FindOrCreateApplicant(ctx, tx, candidates.Applicant{
		FirstName: a.FirstName, LastName: a.LastName, Email: a.Email, Phone: a.Phone,
		Source: careers.CandidateSource(a.Channel),
	})
}

// interviewApplications gives the interviews module the pipeline's
// applications, in its own type.
type interviewApplications struct{ pipeline *pipeline.Module }

func (a interviewApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]interviews.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, ids)
	out := make(map[string]interviews.Application, len(briefs))
	for id, b := range briefs {
		out[id] = interviews.Application{
			ID: b.ID, Stage: b.Stage, JobID: b.JobID, JobTitle: b.JobTitle, CandidateID: b.CandidateID, CandidateName: b.CandidateName,
		}
	}
	return out, err
}

func (a interviewApplications) ReachInterview(ctx context.Context, tx pgx.Tx, id string) error {
	return a.pipeline.ReachInterview(ctx, tx, id)
}

// placementApplications gives the placements module the pipeline's
// applications, with the job's client and hiring contact, in its own type.
type placementApplications struct {
	pipeline *pipeline.Module
	jobs     *jobs.Module
}

func (a placementApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]placements.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	jobIDs := make([]string, 0, len(briefs))
	for _, b := range briefs {
		jobIDs = append(jobIDs, b.JobID)
	}
	jobBriefs, err := a.jobs.Briefs(ctx, tx, jobIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]placements.Application, len(briefs))
	for id, b := range briefs {
		j := jobBriefs[b.JobID]
		out[id] = placements.Application{
			Stage: b.Stage, CandidateID: b.CandidateID, CandidateName: b.CandidateName, JobID: b.JobID, JobTitle: b.JobTitle,
			CompanyID: j.CompanyID, CompanyName: b.CompanyName, ContactID: j.ContactID,
		}
	}
	return out, nil
}

func (a placementApplications) Place(ctx context.Context, tx pgx.Tx, id string) error {
	return a.pipeline.Place(ctx, tx, id)
}

// offerApplications gives the offers module the pipeline's applications, in
// its own type.
type offerApplications struct{ pipeline *pipeline.Module }

func (a offerApplications) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]offers.Application, error) {
	briefs, err := a.pipeline.Briefs(ctx, tx, ids)
	out := make(map[string]offers.Application, len(briefs))
	for id, b := range briefs {
		out[id] = offers.Application{
			Stage: b.Stage, CandidateID: b.CandidateID, CandidateName: b.CandidateName, JobID: b.JobID, JobTitle: b.JobTitle, CompanyName: b.CompanyName,
		}
	}
	return out, err
}

func (a offerApplications) ReachOffer(ctx context.Context, tx pgx.Tx, id string) error {
	return a.pipeline.ReachOffer(ctx, tx, id)
}
