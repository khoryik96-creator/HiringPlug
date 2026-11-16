package companies

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/classification"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// IndustryCatalogue owns stable selections, in the company service's transaction.
type IndustryCatalogue interface {
	SummaryIn(context.Context, pgx.Tx, *classification.Selections, *string) (*string, error)
	LockIn(context.Context, pgx.Tx) error
	ReadIn(context.Context, pgx.Tx, string, string) (classification.Selections, error)
	SaveIn(context.Context, pgx.Tx, string, string, *classification.Selections, *string) (classification.Selections, error)
	// ChoicesIn is the catalogue's version and its industries and subindustries in use (not retired).
	ChoicesIn(context.Context, pgx.Tx) (int, []IndustryChoice, error)
}

// UseIndustries wires the catalogue.
func (m *Module) UseIndustries(c IndustryCatalogue) { m.service.industries = c }
func (s *Service) saveIndustries(ctx context.Context, tx pgx.Tx, id string, d Details) error {
	if s.industries == nil {
		return nil
	}
	_, err := s.industries.SaveIn(ctx, tx, "company", id, d.IndustrySelections, d.Industry)
	return err
}
func (s *Service) readIndustries(ctx context.Context, tx pgx.Tx, c *Company) error {
	if s.industries == nil {
		return nil
	}
	v, e := s.industries.ReadIn(ctx, tx, "company", c.ID)
	c.Details.IndustrySelections = &v
	return e
}

// LegacyIndustries reads this module's scalar compatibility values.
func (m *Module) LegacyIndustries(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := store.New(tx).LegacyIndustries(ctx)
	out := map[string]string{}
	for _, r := range rows {
		out[r.ID] = r.Industry
	}
	return out, err
}

// SetIndustrySummary keeps existing summary consumers and versions current.
func (m *Module) SetIndustrySummary(ctx context.Context, tx pgx.Tx, id string, label *string) error {
	return store.New(tx).SetIndustrySummary(ctx, store.SetIndustrySummaryParams{ID: id, Industry: label})
}

// SetIndustryTerms keeps the words of all the companies' industries and
// subindustries for their text search (all: ids are every company that has
// some; the others' are cleared). Not a change of the company: no version.
func (m *Module) SetIndustryTerms(ctx context.Context, tx pgx.Tx, ids, terms []string, all bool) error {
	q := store.New(tx)
	if all {
		if err := q.ClearIndustryTermsExcept(ctx, ids); err != nil {
			return err
		}
	}
	return q.SetIndustryTerms(ctx, store.SetIndustryTermsParams{Ids: ids, Terms: terms})
}

// SetIndustrySummaries sets many summaries in one statement (labels[i] is ids[i]'s).
func (m *Module) SetIndustrySummaries(ctx context.Context, tx pgx.Tx, ids, labels []string) error {
	return store.New(tx).SetIndustrySummaries(ctx, store.SetIndustrySummariesParams{Ids: ids, Industries: labels})
}

// IndustryEmployerID requires an exact, normalized company name, never a fuzzy first hit.
func (m *Module) IndustryEmployerID(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	v, e := store.New(tx).IndustryEmployerID(ctx, name)
	if database.IsNoRows(e) {
		return "", nil
	}
	return v, e
}
