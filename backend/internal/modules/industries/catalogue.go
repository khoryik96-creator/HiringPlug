package industries

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

//go:embed seed.json
var seedData []byte

type seedCategory struct {
	ID       string         `json:"id"`
	Label    string         `json:"label"`
	Children []seedCategory `json:"subindustries"`
}

func catalogueIn(ctx context.Context, tx pgx.Tx) (api.IndustryCatalogue, error) {
	q := store.New(tx)
	v, err := q.CatalogueVersion(ctx)
	if err != nil {
		return api.IndustryCatalogue{}, err
	}
	rows, err := q.ListIndustries(ctx)
	out := api.IndustryCatalogue{Version: int(v), Items: []api.Industry{}}
	for _, r := range rows {
		out.Items = append(out.Items, api.Industry{Id: r.ID, ParentId: r.ParentID, Label: r.Label, Aliases: r.Aliases, Retired: r.Retired, SourceKey: r.SourceKey, CandidateCount: int(r.CandidateCount), CompanyCount: int(r.CompanyCount)})
	}
	return out, err
}

// ensureIn serializes one-time, per-agency seeding with all subsequent catalogue writes.
func (s *Service) ensureIn(ctx context.Context, tx pgx.Tx) error {
	q := store.New(tx)
	if _, err := q.CatalogueVersion(ctx); err == nil {
		return nil
	} else if !database.IsNoRows(err) {
		return err
	}
	if err := q.EnsureCatalogue(ctx); err != nil {
		return err
	}
	if _, err := q.LockCatalogue(ctx); err != nil {
		return err
	}
	rows, err := q.ListIndustries(ctx)
	if err != nil || len(rows) > 0 {
		return err
	}
	// A catalogue emptied intentionally must not be reseeded.
	v, err := q.CatalogueVersion(ctx)
	if err != nil || v > 1 {
		return err
	}
	var groups []seedCategory
	if err := json.Unmarshal(seedData, &groups); err != nil {
		return fmt.Errorf("industry seed: %w", err)
	}
	for _, g := range groups {
		row, err := q.InsertIndustry(ctx, store.InsertIndustryParams{Label: g.Label, LabelKey: Key(g.Label), Aliases: []string{}, SourceKey: &g.ID})
		if err != nil {
			return err
		}
		for _, c := range g.Children {
			if _, err := q.InsertIndustry(ctx, store.InsertIndustryParams{Label: c.Label, LabelKey: Key(c.Label), ParentID: &row.ID, Aliases: []string{}, SourceKey: &c.ID}); err != nil {
				return err
			}
		}
	}
	if s.options != nil {
		lists, err := s.options.Labels(ctx, tx)
		if err != nil {
			return err
		}
		keys := []string{}
		for k := range lists["industry"] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			label := lists["industry"][k]
			c, err := catalogueIn(ctx, tx)
			if err != nil {
				return err
			}
			item := Match(c.Items, label)
			if item == nil {
				source := "legacy:" + k
				if _, err := q.InsertIndustry(ctx, store.InsertIndustryParams{Label: label, LabelKey: Key(label), Aliases: []string{k}, SourceKey: &source}); err != nil {
					return err
				}
			}
		}
	}
	if s.records == nil {
		return nil
	}
	records, err := s.records.LegacyIndustries(ctx, tx)
	if err != nil {
		return err
	}
	for _, record := range records {
		c, err := catalogueIn(ctx, tx)
		if err != nil {
			return err
		}
		item := Match(c.Items, record.Label)
		if item == nil {
			// Unknown historical labels remain visible and selectable for their existing records only.
			source := "legacy-value:" + Key(record.Label)
			row, err := q.InsertIndustry(ctx, store.InsertIndustryParams{SourceKey: &source, Label: record.Label, LabelKey: Key(record.Label), Aliases: []string{}, Retired: true})
			if err != nil {
				return err
			}
			item = &api.Industry{Id: row.ID}
		}
		if err := assign(ctx, q, record.Kind, record.ID, item.Id, true); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "industry_catalogue.seeded", EntityType: "industry_catalogue", EntityID: "catalogue"})
}
