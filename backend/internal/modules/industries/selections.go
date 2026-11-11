package industries

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
)

func subject(kind, id string) (candidate, company *string) {
	if kind == "candidate" {
		return &id, nil
	}
	return nil, &id
}
func assign(ctx context.Context, q *store.Queries, kind, id, category string, primary bool) error {
	a, b := subject(kind, id)
	return q.AssignIndustry(ctx, store.AssignIndustryParams{CandidateID: a, CompanyID: b, CategoryID: category, IsPrimary: primary})
}
func selectionIn(ctx context.Context, q *store.Queries, kind, id string, v int) (api.IndustrySelections, error) {
	a, b := subject(kind, id)
	rows, err := q.SubjectIndustries(ctx, store.SubjectIndustriesParams{CandidateID: a, CompanyID: b})
	out := api.IndustrySelections{CatalogueVersion: v, Ids: []string{}}
	for _, r := range rows {
		out.Ids = append(out.Ids, r.CategoryID)
		if r.IsPrimary {
			copy := r.CategoryID
			out.PrimaryId = &copy
		}
	}
	slices.Sort(out.Ids)
	return out, err
}
func (s *Service) readIn(ctx context.Context, tx pgx.Tx, kind, id string) (api.IndustrySelections, error) {
	if err := s.ensureIn(ctx, tx); err != nil {
		return api.IndustrySelections{}, err
	}
	v, err := store.New(tx).CatalogueVersion(ctx)
	if err != nil {
		return api.IndustrySelections{}, err
	}
	return selectionIn(ctx, store.New(tx), kind, id, int(v))
}
func (s *Service) saveIn(ctx context.Context, tx pgx.Tx, kind, id string, in *api.IndustrySelections, legacy *string) (api.IndustrySelections, error) {
	if err := s.ensureIn(ctx, tx); err != nil {
		return api.IndustrySelections{}, err
	}
	if _, err := store.New(tx).LockCatalogue(ctx); err != nil {
		return api.IndustrySelections{}, err
	}
	before, err := s.readIn(ctx, tx, kind, id)
	if err != nil {
		return before, err
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return before, err
	}
	items := catalogueMap(c.Items)
	legacyImported := ""
	if in == nil {
		// Existing clients continue to edit a broad scalar; changing it preserves other tags.
		if before.PrimaryId != nil {
			label := root(items[*before.PrimaryId], items).Label
			if legacy != nil && Key(label) == Key(*legacy) {
				return before, nil
			}
		}
		if legacy == nil && before.PrimaryId == nil {
			return before, nil
		}
		copy := before
		copy.PrimaryId = nil
		if legacy != nil {
			item := Match(c.Items, *legacy)
			if item == nil {
				source := "legacy-value:" + Key(*legacy)
				row, e := store.New(tx).InsertIndustry(ctx, store.InsertIndustryParams{SourceKey: &source, Label: *legacy, LabelKey: Key(*legacy), Aliases: []string{}, Retired: true})
				if e != nil {
					return before, e
				}
				created := api.Industry{Id: row.ID, Label: row.Label, Aliases: []string{}, Retired: true}
				items[created.Id] = created
				item = &created
				legacyImported = created.Id
				if e := store.New(tx).BumpCatalogue(ctx); e != nil {
					return before, e
				}
				c.Version++
				copy.CatalogueVersion = c.Version
				if e := audit.Record(ctx, tx, audit.Entry{Action: "industry.legacy_imported", EntityType: "industry", EntityID: created.Id}); e != nil {
					return before, e
				}
			}
			if item.SourceKey != nil && strings.HasPrefix(*item.SourceKey, "legacy-value:") {
				legacyImported = item.Id
			}
			if !slices.Contains(copy.Ids, item.Id) {
				copy.Ids = append(copy.Ids, item.Id)
			}
			copy.PrimaryId = &item.Id
		}
		in = &copy
	}
	if in.CatalogueVersion != c.Version {
		return before, apperr.StaleVersion("industry catalogue")
	}
	if len(in.Ids) > 50 {
		return before, invalid("industrySelections.ids", "use at most 50 categories")
	}
	if in.PrimaryId != nil && !slices.Contains(in.Ids, *in.PrimaryId) {
		return before, invalid("industrySelections.primaryId", "the current category must be selected")
	}
	seen := map[string]bool{}
	for _, id := range in.Ids {
		item, ok := items[id]
		if !ok {
			return before, invalid("industrySelections.ids", "a selected category no longer exists; reload the catalogue")
		}
		if seen[id] {
			return before, invalid("industrySelections.ids", "remove duplicate selections")
		}
		seen[id] = true
		if !active(item, items) && !slices.Contains(before.Ids, id) && id != legacyImported {
			return before, invalid("industrySelections.ids", "retired categories cannot be newly selected")
		}
	}
	if err := writeSelections(ctx, store.New(tx), kind, id, *in); err != nil {
		return before, err
	}
	var summary *string
	if in.PrimaryId != nil {
		label := root(items[*in.PrimaryId], items).Label
		summary = &label
	}
	if s.records != nil {
		if err := s.records.SetIndustrySummary(ctx, tx, kind, id, summary); err != nil {
			return before, err
		}
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: kind + ".industries_saved", EntityType: kind, EntityID: id, Changes: map[string]any{"categoryCount": len(in.Ids)}}); err != nil {
		return before, err
	}
	return selectionIn(ctx, store.New(tx), kind, id, c.Version)
}
func writeSelections(ctx context.Context, q *store.Queries, kind, id string, in api.IndustrySelections) error {
	a, b := subject(kind, id)
	if err := q.ClearSubjectIndustries(ctx, store.ClearSubjectIndustriesParams{CandidateID: a, CompanyID: b}); err != nil {
		return err
	}
	for _, v := range in.Ids {
		if err := assign(ctx, q, kind, id, v, in.PrimaryId != nil && v == *in.PrimaryId); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) replaceIn(ctx context.Context, tx pgx.Tx, affected map[string]bool, replacement *api.Industry) error {
	q := store.New(tx)
	rows, err := q.AllIndustryAssignments(ctx)
	if err != nil {
		return err
	}
	type group struct {
		kind, id string
		in       api.IndustrySelections
	}
	groups := map[string]*group{}
	for _, r := range rows {
		kind, id := "candidate", r.CandidateID
		if id == nil {
			kind, id = "company", r.CompanyID
		}
		k := kind + ":" + *id
		g := groups[k]
		if g == nil {
			g = &group{kind: kind, id: *id}
			groups[k] = g
		}
		cat := r.CategoryID
		if affected[cat] {
			if replacement == nil {
				continue
			}
			cat = replacement.Id
		}
		if !slices.Contains(g.in.Ids, cat) {
			g.in.Ids = append(g.in.Ids, cat)
		}
		if r.IsPrimary {
			copy := cat
			g.in.PrimaryId = &copy
		}
	}
	for _, g := range groups {
		if err := writeSelections(ctx, q, g.kind, g.id, g.in); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) refreshSummaries(ctx context.Context, tx pgx.Tx) error {
	if s.records == nil {
		return nil
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return err
	}
	items := catalogueMap(c.Items)
	rows, err := store.New(tx).AllIndustryAssignments(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !r.IsPrimary {
			continue
		}
		kind, id := "candidate", r.CandidateID
		if id == nil {
			kind, id = "company", r.CompanyID
		}
		label := root(items[r.CategoryID], items).Label
		if err := s.records.SetIndustrySummary(ctx, tx, kind, *id, &label); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) candidateIDsIn(ctx context.Context, tx pgx.Tx, values []string) ([]string, error) {
	if err := s.ensureIn(ctx, tx); err != nil {
		return nil, err
	}
	c, err := catalogueIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	items := catalogueMap(c.Items)
	for _, v := range values {
		var item *api.Industry
		if i, ok := items[v]; ok {
			copy := i
			item = &copy
		} else {
			item = Match(c.Items, v)
		}
		if item == nil {
			continue
		}
		ids = append(ids, item.Id)
		for _, i := range c.Items {
			if i.ParentId != nil && *i.ParentId == item.Id {
				ids = append(ids, i.Id)
			}
		}
	}
	return store.New(tx).CandidatesInIndustries(ctx, ids)
}
