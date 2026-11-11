package industries

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/industries/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Service runs catalogue use cases in one tenant-scoped transaction.
type Service struct {
	pool    *pgxpool.Pool
	records Records
	options LegacyOptions
}

func (s *Service) List(ctx context.Context, p authz.Principal) (api.IndustryCatalogue, error) {
	var out api.IndustryCatalogue
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.ensureIn(ctx, tx); err != nil {
			return err
		}
		var err error
		out, err = catalogueIn(ctx, tx)
		return err
	})
	if !p.Can(api.PermissionSettingsManage) {
		for i := range out.Items {
			out.Items[i].CandidateCount = 0
			out.Items[i].CompanyCount = 0
		}
	}
	return out, err
}
func (s *Service) Edit(ctx context.Context, p authz.Principal, id string, in api.IndustryEdit) (api.IndustryCatalogue, error) {
	var out api.IndustryCatalogue
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.ensureIn(ctx, tx); err != nil {
			return err
		}
		if _, err := store.New(tx).LockCatalogue(ctx); err != nil {
			return err
		}
		c, err := catalogueIn(ctx, tx)
		if err != nil {
			return err
		}
		if in.CatalogueVersion != c.Version {
			return apperr.StaleVersion("industry catalogue")
		}
		checked, err := CheckEdit(c.Items, id, in)
		if err != nil {
			return err
		}
		q := store.New(tx)
		if id == "" {
			row, err := q.InsertIndustry(ctx, store.InsertIndustryParams{Label: checked.Label, LabelKey: Key(checked.Label), Aliases: checked.Aliases, ParentID: checked.ParentId, Retired: checked.Retired})
			if err != nil {
				return err
			}
			id = row.ID
		} else if err := q.UpdateIndustry(ctx, store.UpdateIndustryParams{ID: id, Label: checked.Label, LabelKey: Key(checked.Label), Aliases: checked.Aliases, ParentID: checked.ParentId, Retired: checked.Retired}); err != nil {
			return err
		}
		if checked.Retired {
			if err := q.RetireIndustry(ctx, id); err != nil {
				return err
			}
		}
		if err := q.BumpCatalogue(ctx); err != nil {
			return err
		}
		if err := s.refreshSummaries(ctx, tx); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "industry.saved", EntityType: "industry", EntityID: id}); err != nil {
			return err
		}
		out, err = catalogueIn(ctx, tx)
		return err
	})
	return out, err
}
func (s *Service) Remove(ctx context.Context, p authz.Principal, id string, in api.RemoveIndustryJSONRequestBody) (api.IndustryCatalogue, error) {
	var out api.IndustryCatalogue
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.ensureIn(ctx, tx); err != nil {
			return err
		}
		if _, err := store.New(tx).LockCatalogue(ctx); err != nil {
			return err
		}
		c, err := catalogueIn(ctx, tx)
		if err != nil {
			return err
		}
		if in.CatalogueVersion != c.Version {
			return apperr.StaleVersion("industry catalogue")
		}
		items := catalogueMap(c.Items)
		target, ok := items[id]
		if !ok {
			return apperr.NotFound("industry")
		}
		q := store.New(tx)
		if string(in.Action) == "retire" {
			if err := q.RetireIndustry(ctx, id); err != nil {
				return err
			}
		} else {
			affected := map[string]bool{id: true}
			for _, i := range c.Items {
				if i.ParentId != nil && *i.ParentId == id {
					affected[i.Id] = true
				}
			}
			var replacement *api.Industry
			if in.ReplacementId != nil {
				r, ok := items[*in.ReplacementId]
				if !ok || affected[r.Id] || !active(r, items) {
					return invalid("replacementId", "choose an active category outside this group")
				}
				replacement = &r
			}
			if replacement == nil && (target.CandidateCount > 0 || target.CompanyCount > 0) {
				return invalid("replacementId", "this category is in use; choose a replacement or retire it")
			}
			if err := s.replaceIn(ctx, tx, affected, replacement); err != nil {
				return err
			}
			for _, i := range c.Items {
				if i.ParentId != nil && *i.ParentId == id {
					if err := q.DeleteIndustry(ctx, i.Id); err != nil {
						return err
					}
				}
			}
			if err := q.DeleteIndustry(ctx, id); err != nil {
				return err
			}
		}
		if err := q.BumpCatalogue(ctx); err != nil {
			return err
		}
		if err := s.refreshSummaries(ctx, tx); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "industry." + string(in.Action), EntityType: "industry", EntityID: id}); err != nil {
			return err
		}
		out, err = catalogueIn(ctx, tx)
		return err
	})
	return out, err
}
