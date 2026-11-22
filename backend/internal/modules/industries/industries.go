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
	SetIndustrySummaries(ctx context.Context, tx pgx.Tx, kind string, ids, labels []string) error
	// SetIndustryTerms keeps the words of every industry and subindustry a
	// record has (for its text search); with all, ids are every record that
	// has some, and the others' are cleared.
	SetIndustryTerms(ctx context.Context, tx pgx.Tx, kind string, ids, terms []string, all bool) error
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

// LockIn takes the catalogue lock, shared, before a record is edited: record
// saves run side by side and wait only for a catalogue edit, which takes it
// exclusively; taking it first avoids lock inversion.
func (m *Module) LockIn(ctx context.Context, tx pgx.Tx) error {
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return err
	}
	_, err := store.New(tx).ShareCatalogue(ctx)
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
	return m.service.moveSubject(ctx, tx, "candidate", fromID, toID)
}

// MoveCompany gives the company kept in a merge the duplicate's industries
// too (its own current one stays current, or takes the duplicate's).
func (m *Module) MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return m.service.moveSubject(ctx, tx, "company", fromID, toID)
}

func (s *Service) moveSubject(ctx context.Context, tx pgx.Tx, kind, fromID, toID string) error {
	if err := s.ensureIn(ctx, tx); err != nil {
		return err
	}
	if _, err := store.New(tx).ShareCatalogue(ctx); err != nil {
		return err
	}
	from, e := s.readIn(ctx, tx, kind, fromID)
	if e != nil {
		return e
	}
	to, e := s.readIn(ctx, tx, kind, toID)
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
	q := store.New(tx)
	if err := writeSelections(ctx, q, kind, toID, to); err != nil {
		return err
	}
	if kind != "company" || s.records == nil || len(from.Ids) == 0 {
		return nil
	}
	// The kept company's summary and search words follow its industries.
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return err
	}
	items := catalogueMap(c.Items)
	var label *string
	if to.PrimaryId != nil {
		l := root(items[*to.PrimaryId], items).Label
		label = &l
	}
	if err := s.records.SetIndustrySummary(ctx, tx, kind, toID, label); err != nil {
		return err
	}
	return s.records.SetIndustryTerms(ctx, tx, kind, []string{toID}, []string{termsOf(to.Ids, items)}, false)
}

// Choice is an industry or subindustry in use: its id, its words
// ("Financial Services > Banking") and its industry's words.
type Choice struct{ ID, Label, Root string }

// ChoicesIn returns the catalogue's version and its industries and
// subindustries in use (not retired), industries first, each followed by its
// subindustries, in the caller's transaction.
func (m *Module) ChoicesIn(ctx context.Context, tx pgx.Tx) (int, []Choice, error) {
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return 0, nil, err
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return 0, nil, err
	}
	items := catalogueMap(c.Items)
	var out []Choice
	for _, top := range c.Items {
		if top.ParentId != nil || !active(top, items) {
			continue
		}
		out = append(out, Choice{ID: top.Id, Label: top.Label, Root: top.Label})
		for _, sub := range c.Items {
			if sub.ParentId != nil && *sub.ParentId == top.Id && active(sub, items) {
				out = append(out, Choice{ID: sub.Id, Label: top.Label + " > " + sub.Label, Root: top.Label})
			}
		}
	}
	return c.Version, out, nil
}

// DisplayLabelsIn gives other modules readable labels without sharing SQL ownership.
func (m *Module) DisplayLabelsIn(ctx context.Context, tx pgx.Tx, ids []string) ([]string, error) {
	if err := m.service.ensureIn(ctx, tx); err != nil {
		return nil, err
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	items := catalogueMap(c.Items)
	out := []string{}
	for _, id := range ids {
		if i, ok := items[id]; ok {
			label := i.Label
			if i.ParentId != nil {
				label = items[*i.ParentId].Label + " / " + label
			}
			out = append(out, label)
		}
	}
	return out, nil
}
