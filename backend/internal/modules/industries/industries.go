// Package industries owns the agency catalogue, selections and employer-sector rules.
package industries

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/classification"
)

// LegacyIndustry is an existing scalar selection kept during the catalogue migration.
type LegacyIndustry struct{ Kind, ID, Label string }

// Records keeps candidate/company SQL in the module that owns those tables.
type Records interface {
	LegacyIndustries(context.Context, pgx.Tx) ([]LegacyIndustry, error)
	SetIndustrySummary(context.Context, pgx.Tx, string, string, *string) error
	EmployerEvidence(context.Context, pgx.Tx, string) (Evidence, error)
	VerifiedEmployer(context.Context, pgx.Tx, string) (string, error)
}

// LegacyOptions preserves agency-specific options on the first catalogue read.
type LegacyOptions interface {
	Labels(context.Context, pgx.Tx) (map[string]map[string]string, error)
}

// Module is the public industry API. All functions use the caller's tenant transaction.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New constructs the module, before its record interfaces are wired.
func New(pool *pgxpool.Pool) *Module {
	s := &Service{pool: pool}
	return &Module{service: s, HTTP: &HTTP{svc: s}}
}

// UseRecords wires the record owners without module imports.
func (m *Module) UseRecords(r Records, o LegacyOptions) { m.service.records = r; m.service.options = o }

// ReadIn reads selections (including retired values) in the caller's transaction.
func (m *Module) ReadIn(ctx context.Context, tx pgx.Tx, kind, id string) (classification.Selections, error) {
	v, e := m.service.readIn(ctx, tx, kind, id)
	return classification.Selections(v), e
}

// SaveIn saves an optional selection input, or synchronises a changed legacy scalar.
func (m *Module) SaveIn(ctx context.Context, tx pgx.Tx, kind, id string, in *classification.Selections, legacy *string) (classification.Selections, error) {
	var input *api.IndustrySelections
	if in != nil {
		v := api.IndustrySelections(*in)
		input = &v
	}
	v, e := m.service.saveIn(ctx, tx, kind, id, input, legacy)
	return classification.Selections(v), e
}

// CandidateIDsIn returns candidate IDs matching catalogue IDs or canonical legacy labels.
func (m *Module) CandidateIDsIn(ctx context.Context, tx pgx.Tx, values []string) ([]string, error) {
	return m.service.candidateIDsIn(ctx, tx, values)
}

// LockIn takes the catalogue lock before a record is edited, avoiding lock inversion.
func (m *Module) LockIn(ctx context.Context, tx pgx.Tx) error {
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return err
	}
	_, err := store.New(tx).LockCatalogue(ctx)
	return err
}

// SummaryIn resolves the selected primary category before owner-field validation.
func (m *Module) SummaryIn(ctx context.Context, tx pgx.Tx, in *classification.Selections, legacy *string) (*string, error) {
	if in == nil {
		return legacy, nil
	}
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return nil, err
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	if in.PrimaryId == nil {
		return nil, nil //nolint:nilnil // no primary industry is a valid explicit choice
	}
	items := catalogueMap(c.Items)
	i, ok := items[*in.PrimaryId]
	if !ok {
		return nil, invalid("industrySelections.primaryId", "choose an existing category")
	}
	label := root(i, items).Label
	return &label, nil
}

// MoveCandidate preserves both records' classifications during duplicate merging.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	if err := m.LockIn(ctx, tx); err != nil {
		return err
	}
	from, e := m.service.readIn(ctx, tx, "candidate", fromID)
	if e != nil {
		return e
	}
	to, e := m.service.readIn(ctx, tx, "candidate", toID)
	if e != nil {
		return e
	}
	for _, id := range from.Ids {
		if !slices.Contains(to.Ids, id) {
			to.Ids = append(to.Ids, id)
		}
	}
	if to.PrimaryId == nil {
		to.PrimaryId = from.PrimaryId
	}
	if err := writeSelections(ctx, store.New(tx), "candidate", toID, to); err != nil {
		return err
	}
	return nil
}
