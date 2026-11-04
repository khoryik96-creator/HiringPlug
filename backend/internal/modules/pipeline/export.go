package pipeline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
)

// Exported is an application as it goes into a copy of a candidate's data.
type Exported struct {
	JobTitle, CompanyName, Stage string
	Reason                       *string
	AddedAt, StageChangedAt      time.Time
}

// ForExport returns a candidate's applications with their jobs, newest
// first, in the caller's transaction.
func (m *Module) ForExport(ctx context.Context, tx pgx.Tx, candidateID string) ([]Exported, error) {
	rows, err := store.New(tx).ListCandidateApplications(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	views, err := m.service.views(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]Exported, len(views))
	for i, v := range views {
		out[i] = Exported{JobTitle: v.JobTitle, CompanyName: v.CompanyName, Stage: v.Stage, Reason: v.Reason, AddedAt: v.CreatedAt, StageChangedAt: v.StageChangedAt}
	}
	return out, nil
}
