package identity

// Each person's own settings, such as the columns of their candidate list.
// The database lets a person see and change only their own.

import (
	"context"
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

// Preferences returns the signed-in person's settings (null columns: the default ones).
func (m *Module) Preferences(ctx context.Context, p authz.Principal) (api.Preferences, error) {
	var out api.Preferences
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		cols, err := store.New(tx).GetPreferences(ctx)
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		out = preferencesOf(cols)
		return nil
	})
	return out, err
}

// SetPreferences saves the signed-in person's settings.
func (m *Module) SetPreferences(ctx context.Context, p authz.Principal, in api.Preferences) (api.Preferences, error) {
	var cols []string // nil: back to the default
	if v, err := in.CandidateColumns.Get(); err == nil {
		cols = make([]string, 0, len(v))
		for i, c := range v {
			if !c.Valid() {
				return api.Preferences{}, apperr.Validation(apperr.FieldErrors{{Field: fmt.Sprintf("candidateColumns[%d]", i), Message: "is not a column"}})
			}
			if slices.Contains(cols, string(c)) {
				return api.Preferences{}, apperr.Validation(apperr.FieldErrors{{Field: fmt.Sprintf("candidateColumns[%d]", i), Message: "is there twice"}})
			}
			cols = append(cols, string(c))
		}
	}
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if cols == nil {
			err = q.ResetCandidateColumns(ctx)
		} else {
			err = q.SetCandidateColumns(ctx, cols)
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.preferences_changed", EntityType: "user", EntityID: p.UserID,
			Changes: map[string]any{"candidateColumns": cols}})
	})
	if err != nil {
		return api.Preferences{}, err
	}
	return preferencesOf(cols), nil
}

func preferencesOf(cols []string) api.Preferences {
	if cols == nil {
		return api.Preferences{CandidateColumns: nullable.NewNullNullable[[]api.CandidateColumn]()}
	}
	out := make([]api.CandidateColumn, 0, len(cols))
	for _, c := range cols {
		if col := api.CandidateColumn(c); col.Valid() { // a column since retired is dropped
			out = append(out, col)
		}
	}
	return api.Preferences{CandidateColumns: nullable.NewNullableWithValue(out)}
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
