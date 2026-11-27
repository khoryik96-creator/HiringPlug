package app

// Adapters giving the reports module other modules' records, in its own types.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/reports"
)

type reportApplications struct{ pipeline *pipeline.Module }

func (a reportApplications) Facts(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.Application, error) {
	facts, err := a.pipeline.Facts(ctx, tx, from, to)
	out := make([]reports.Application, len(facts))
	for i, f := range facts {
		out[i] = reports.Application{CandidateID: f.CandidateID, Channel: f.Channel, Stages: f.Stages, Categories: f.Categories}
	}
	return out, err
}

func (a reportApplications) Categories(ctx context.Context, tx pgx.Tx) ([]reports.Category, error) {
	cats, err := a.pipeline.CategoriesIn(ctx, tx)
	out := make([]reports.Category, len(cats))
	for i, c := range cats {
		out[i] = reports.Category{Key: c.Key, Label: c.Label, Kind: c.Kind}
	}
	return out, err
}

func (a reportApplications) FirstPlaced(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.Placed, error) {
	placed, err := a.pipeline.FirstPlaced(ctx, tx, from, to)
	out := make([]reports.Placed, len(placed))
	for i, p := range placed {
		out[i] = reports.Placed{JobID: p.JobID, CreatedAt: p.CreatedAt, PlacedAt: p.PlacedAt}
	}
	return out, err
}

func jobFirsts(firsts []pipeline.JobFirst, err error) ([]reports.JobFirst, error) {
	out := make([]reports.JobFirst, len(firsts))
	for i, f := range firsts {
		out[i] = reports.JobFirst{JobID: f.JobID, At: f.At}
	}
	return out, err
}

func (a reportApplications) FirstCVs(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.JobFirst, error) {
	return jobFirsts(a.pipeline.FirstSubmitted(ctx, tx, from, to))
}

func (a reportApplications) FirstPlacements(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]reports.JobFirst, error) {
	return jobFirsts(a.pipeline.FirstPlacedJobs(ctx, tx, from, to))
}

func (a reportApplications) CategoriesReached(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]reports.Reach, error) {
	reached, err := a.pipeline.CategoriesReached(ctx, tx, ids)
	out := make(map[string][]reports.Reach, len(reached))
	for app, rs := range reached {
		for _, r := range rs {
			out[app] = append(out[app], reports.Reach{Category: r.Category, At: r.At})
		}
	}
	return out, err
}

type reportSources struct{ candidates *candidates.Module }

func (a reportSources) Sources(ctx context.Context, tx pgx.Tx, ids []string) (map[string]*string, error) {
	briefs, err := a.candidates.Briefs(ctx, tx, ids)
	out := make(map[string]*string, len(briefs))
	for id, b := range briefs {
		out[id] = b.Source
	}
	return out, err
}

type reportJobs struct{ jobs *jobs.Module }

func (a reportJobs) Jobs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]reports.Job, error) {
	briefs, err := a.jobs.Briefs(ctx, tx, ids)
	out := make(map[string]reports.Job, len(briefs))
	for id, b := range briefs {
		out[id] = reports.Job{Title: b.Title, Opened: b.CreatedAt, OwnerID: b.OwnerID}
	}
	return out, err
}

type reportPlacements struct{ placements *placements.Module }

func (a reportPlacements) Facts(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]reports.Placement, error) {
	facts, err := a.placements.Facts(ctx, tx, fromDay, toDay)
	out := make([]reports.Placement, len(facts))
	for i, f := range facts {
		out[i] = reports.Placement{RecruiterID: f.RecruiterID, Status: f.Status, FeeAmount: f.FeeAmount, FeeCurrency: f.FeeCurrency, StartDate: f.StartDate}
	}
	return out, err
}
