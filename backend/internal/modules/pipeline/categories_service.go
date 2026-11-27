package pipeline

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// categories reads the company's categories in tx (the standard ones until
// it saves its own) and their version ("0" for the standard ones).
func categories(ctx context.Context, tx pgx.Tx) ([]Category, string, error) {
	rows, err := store.New(tx).ListStageCategories(ctx)
	if err != nil || len(rows) == 0 {
		return StandardCategories(), "0", err
	}
	out := make([]Category, len(rows))
	var latest int64
	for i, r := range rows {
		out[i] = Category{Key: r.Key, Label: r.Label, Kind: r.Kind}
		latest = max(latest, r.UpdatedAt.UnixMicro())
	}
	return out, strconv.FormatInt(latest, 10), nil
}

// appCategory is the category of an application on jobID at stage (and
// step), in tx.
func (s *Service) appCategory(ctx context.Context, tx pgx.Tx, jobID, stage string, step *string) (*string, error) {
	if IsClosed(stage) {
		return nil, nil
	}
	js, err := s.jobStages(ctx, tx, jobID)
	if err != nil {
		return nil, err
	}
	cats, _, err := categories(ctx, tx)
	return AppCategory(js.Steps, cats, stage, step), err
}

// Categories returns the company's stage categories, in order, and their version.
func (s *Service) Categories(ctx context.Context, p authz.Principal) ([]Category, string, error) {
	var out []Category
	var version string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, version, err = categories(ctx, tx)
		return err
	})
	return out, version, err
}

// CategoriesIn returns the company's categories in the caller's transaction
// (reports).
func (m *Module) CategoriesIn(ctx context.Context, tx pgx.Tx) ([]Category, error) {
	out, _, err := categories(ctx, tx)
	return out, err
}

// SetCategories saves the company's categories (version: the one loaded).
// One taken away must be unused by any stage or open application.
func (s *Service) SetCategories(ctx context.Context, p authz.Principal, version string, in []Category) ([]Category, string, error) {
	var out []Category
	var outVersion string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockStageCategories(ctx); err != nil {
			return err
		}
		before, current, err := categories(ctx, tx)
		if err != nil {
			return err
		}
		if current != version {
			return apperr.New(api.ErrorCodeCONFLICTSTALEVERSION, "The categories were changed by someone else. Reload them and try again.")
		}
		cats, errs := NormalizeCategories(in, before)
		if err := errs.Err(); err != nil {
			return err
		}
		if gone := Removed(before, cats); len(gone) > 0 {
			use, err := q.StageCategoryUse(ctx, gone)
			if err != nil {
				return err
			}
			if use.Stages > 0 || use.Applications > 0 {
				return apperr.New(api.ErrorCodeCATEGORYINUSE, "A category you took away is still used: move its stages and candidates to another first.").
					WithDetail("stages", use.Stages).WithDetail("applications", use.Applications)
			}
		}
		if err := q.DeleteStageCategories(ctx); err != nil {
			return err
		}
		labels := make([]string, len(cats))
		for i, c := range cats {
			labels[i] = c.Label
			if err := q.InsertStageCategory(ctx, store.InsertStageCategoryParams{Key: c.Key, Label: c.Label, Kind: c.Kind, Position: int32(i)}); err != nil { //nolint:gosec // at most 40
				return err
			}
		}
		if out, outVersion, err = categories(ctx, tx); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "stage_categories.changed", EntityType: "stage_category", EntityID: "all",
			Changes: map[string]any{"categories": labels, "removed": Removed(before, cats)}})
	})
	return out, outVersion, err
}

// ListStageCategories handles GET /v1/stage-categories.
func (h *HTTP) ListStageCategories(ctx context.Context, _ api.ListStageCategoriesRequestObject) (api.ListStageCategoriesResponseObject, error) {
	cats, version, err := h.svc.Categories(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.ListStageCategories200JSONResponse(categoriesToAPI(cats, version)), nil
}

// SetStageCategories handles PUT /v1/stage-categories.
func (h *HTTP) SetStageCategories(ctx context.Context, req api.SetStageCategoriesRequestObject) (api.SetStageCategoriesResponseObject, error) {
	in := make([]Category, len(req.Body.Items))
	for i, c := range req.Body.Items {
		in[i] = Category{Label: c.Label, Kind: string(c.Kind)}
		if c.Key != nil {
			in[i].Key = *c.Key
		}
	}
	cats, version, err := h.svc.SetCategories(ctx, authz.MustFromContext(ctx), req.Body.Version, in)
	if err != nil {
		return nil, err
	}
	return api.SetStageCategories200JSONResponse(categoriesToAPI(cats, version)), nil
}

func categoriesToAPI(cats []Category, version string) api.StageCategories {
	out := api.StageCategories{Items: make([]api.StageCategory, len(cats)), Version: version}
	for i, c := range cats {
		out.Items[i] = api.StageCategory{Key: c.Key, Label: c.Label, Kind: api.OpenStage(c.Kind)}
	}
	return out
}
