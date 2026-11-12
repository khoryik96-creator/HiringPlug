package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/classification"
)

// IndustryCatalogue owns stable industry assignments, in this service's transaction.
type IndustryCatalogue interface {
	SummaryIn(context.Context, pgx.Tx, *classification.Selections, *string) (*string, error)
	LockIn(context.Context, pgx.Tx) error
	ReadIn(context.Context, pgx.Tx, string, string) (classification.Selections, error)
	SaveIn(context.Context, pgx.Tx, string, string, *classification.Selections, *string) (classification.Selections, error)
	CandidateIDsIn(context.Context, pgx.Tx, []string) ([]string, error)
}

// UseIndustries wires the shared catalogue.
func (m *Module) UseIndustries(c IndustryCatalogue) { m.service.industries = c }
func (s *Service) saveIndustries(ctx context.Context, tx pgx.Tx, id string, p Profile) error {
	if s.industries == nil {
		return nil
	}
	_, err := s.industries.SaveIn(ctx, tx, "candidate", id, p.IndustrySelections, p.Industry)
	return err
}

// LegacyIndustries reads the candidate-owned scalar compatibility values.
func (m *Module) LegacyIndustries(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := store.New(tx).LegacyIndustries(ctx)
	out := map[string]string{}
	for _, r := range rows {
		out[r.ID] = r.Industry
	}
	return out, err
}

// SetIndustrySummary updates the scalar compatibility view, retaining optimistic versions.
func (m *Module) SetIndustrySummary(ctx context.Context, tx pgx.Tx, id string, label *string) error {
	return store.New(tx).SetIndustrySummary(ctx, store.SetIndustrySummaryParams{ID: id, Industry: label})
}

// SetIndustrySummaries sets many summaries in one statement (labels[i] is ids[i]'s).
func (m *Module) SetIndustrySummaries(ctx context.Context, tx pgx.Tx, ids, labels []string) error {
	return store.New(tx).SetIndustrySummaries(ctx, store.SetIndustrySummariesParams{Ids: ids, Industries: labels})
}
