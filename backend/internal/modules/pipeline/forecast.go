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
