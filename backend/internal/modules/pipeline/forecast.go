package pipeline

// What reports read: moves to a stage by a person (CVs sent are moves to
// submitted), and the applications now at some stages (the revenue forecast).

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
)

// MovedBy is an application moved to a stage by a person.
type MovedBy struct {
	By, CandidateID string
	At              time.Time
}

// MovesTo returns the moves to stage in [from, to) made by a person (not by
// the app), in the caller's transaction.
func (m *Module) MovesTo(ctx context.Context, tx pgx.Tx, stage string, from, to time.Time) ([]MovedBy, error) {
	rows, err := store.New(tx).MovesToStageBetween(ctx, store.MovesToStageBetweenParams{Stage: stage, FromTime: from, ToTime: to})
	out := make([]MovedBy, len(rows))
	for i, r := range rows {
		out[i] = MovedBy{By: r.ChangedBy, CandidateID: r.CandidateID, At: r.ChangedAt}
	}
	return out, err
}

// CategoryMove is a user moving an application into a stage of a category.
type CategoryMove struct {
	By, ApplicationID, CandidateID, Category string
	At                                       time.Time
}

// MovesToCategory returns the moves into a stage category (from another
// category or none) from one instant to another, in the caller's
// transaction. A move recorded without a category (before categories) is in
// its stage's first.
func (m *Module) MovesToCategory(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CategoryMove, error) {
	rows, err := store.New(tx).MovesToCategoryBetween(ctx, store.MovesToCategoryBetweenParams{FromTime: from, ToTime: to})
	if err != nil {
		return nil, err
	}
	cats, _, err := categories(ctx, tx)
	if err != nil {
		return nil, err
	}
	first := firstOfKind(cats)
	var out []CategoryMove
	for _, r := range rows {
		into := categoryOf(r.ToStage, r.ToCategory, first)
		was := ""
		if r.FromStage != nil {
			was = categoryOf(*r.FromStage, r.FromCategory, first)
		}
		if into == "" || into == was {
			continue
		}
		out = append(out, CategoryMove{By: r.ChangedBy, ApplicationID: r.ApplicationID, CandidateID: r.CandidateID, Category: into, At: r.ChangedAt})
	}
	return out, nil
}

// AtStages returns the applications now at any of stages (at most 5,000),
// with their job and candidate, in the caller's transaction.
func (m *Module) AtStages(ctx context.Context, tx pgx.Tx, stages []string) ([]Brief, error) {
	rows, err := store.New(tx).ApplicationsAtStages(ctx, stages)
	if err != nil {
		return nil, err
	}
	views, err := m.service.views(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]Brief, len(views))
	for i, v := range views {
		out[i] = Brief{ID: v.ID, Stage: v.Stage, JobID: v.JobID, JobTitle: v.JobTitle, CompanyName: v.CompanyName, CandidateID: v.CandidateID, CandidateName: v.CandidateName}
	}
	return out, nil
}
