package identity

// Each person's own settings, such as the columns of their candidate list.
// The database lets a person see and change only their own.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Preferences returns the signed-in person's settings (null: the defaults).
func (m *Module) Preferences(ctx context.Context, p authz.Principal) (api.Preferences, error) {
	var out api.Preferences
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetPreferences(ctx)
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		out = preferencesOf(row)
		return nil
	})
	return out, err
}

// validList checks a list of choices: each known, none twice. nil means
// "the default".
func validList[T ~string](field string, in []T, valid func(T) bool) ([]string, error) {
	out := make([]string, 0, len(in))
	for i, v := range in {
		if !valid(v) {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: fmt.Sprintf("%s[%d]", field, i), Message: "is not one of the choices"}})
		}
		if slices.Contains(out, string(v)) {
			return nil, apperr.Validation(apperr.FieldErrors{{Field: fmt.Sprintf("%s[%d]", field, i), Message: "is there twice"}})
		}
		out = append(out, string(v))
	}
	return out, nil
}

// SetPreferences saves the settings sent (one left out keeps its value; null
// is back to the default).
func (m *Module) SetPreferences(ctx context.Context, p authz.Principal, in api.Preferences) (api.Preferences, error) {
	var cols, blocks []string
	var err error
	if v, e := in.CandidateColumns.Get(); e == nil {
		if cols, err = validList("candidateColumns", v, api.CandidateColumn.Valid); err != nil {
			return api.Preferences{}, err
		}
	}
	if v, e := in.DashboardBlocks.Get(); e == nil {
		if blocks, err = validList("dashboardBlocks", v, api.DashboardBlock.Valid); err != nil {
			return api.Preferences{}, err
		}
	}
	var out api.Preferences
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		changes := map[string]any{}
		if in.CandidateColumns.IsSpecified() {
			if cols == nil {
				err = q.ResetCandidateColumns(ctx)
			} else {
				err = q.SetCandidateColumns(ctx, cols)
			}
			if err != nil {
				return err
			}
			changes["candidateColumns"] = cols
		}
		if in.DashboardBlocks.IsSpecified() {
			if err := q.SetDashboardBlocks(ctx, blocks); err != nil {
				return err
			}
			changes["dashboardBlocks"] = blocks
		}
		if in.Favourites != nil {
			fav := cleanFavourites(*in.Favourites)
			raw, err := json.Marshal(fav)
			if err != nil {
				return err
			}
			if err := q.SetFavourites(ctx, raw); err != nil {
				return err
			}
			changes["favourites"] = fav
		}
		row, err := q.GetPreferences(ctx)
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		out = preferencesOf(row)
		return audit.Record(ctx, tx, audit.Entry{Action: "user.preferences_changed", EntityType: "user", EntityID: p.UserID, Changes: changes})
	})
	if err != nil {
		return api.Preferences{}, err
	}
	return out, nil
}

func preferencesOf(row store.GetPreferencesRow) api.Preferences {
	out := api.Preferences{
		CandidateColumns: nullable.NewNullNullable[[]api.CandidateColumn](),
		DashboardBlocks:  nullable.NewNullNullable[[]api.DashboardBlock](),
	}
	fav := favouritesOf(row.Favourites)
	out.Favourites = &fav
	if row.CandidateColumns != nil {
		cols := make([]api.CandidateColumn, 0, len(row.CandidateColumns))
		for _, c := range row.CandidateColumns {
			if col := api.CandidateColumn(c); col.Valid() { // a column since retired is dropped
				cols = append(cols, col)
			}
		}
		out.CandidateColumns.Set(cols)
	}
	if row.DashboardBlocks != nil {
		blocks := make([]api.DashboardBlock, 0, len(row.DashboardBlocks))
		for _, b := range row.DashboardBlocks {
			if block := api.DashboardBlock(b); block.Valid() {
				blocks = append(blocks, block)
			}
		}
		out.DashboardBlocks.Set(blocks)
	}
	return out
}

// GetMyPreferences handles GET /v1/me/preferences.
func (h *HTTP) GetMyPreferences(ctx context.Context, _ api.GetMyPreferencesRequestObject) (api.GetMyPreferencesResponseObject, error) {
	out, err := h.m.Preferences(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetMyPreferences200JSONResponse(out), nil
}

// PutMyPreferences handles PUT /v1/me/preferences.
func (h *HTTP) PutMyPreferences(ctx context.Context, r api.PutMyPreferencesRequestObject) (api.PutMyPreferencesResponseObject, error) {
	out, err := h.m.SetPreferences(ctx, authz.MustFromContext(ctx), *r.Body)
	if err != nil {
		return nil, err
	}
	return api.PutMyPreferences200JSONResponse(out), nil
}
