package identity

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// AgencyName tidies an agency's name (spaces collapsed) and checks it.
func AgencyName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len([]rune(name)) > 200 {
		return "", apperr.Validation(apperr.FieldErrors{{Field: "name", Message: "between 1 and 200 characters"}})
	}
	return name, nil
}

// RenameAgency renames the signed-in user's agency (its careers page address stays).
func (m *Module) RenameAgency(ctx context.Context, p authz.Principal, name string) (api.Tenant, error) {
	name, err := AgencyName(name)
	if err != nil {
		return api.Tenant{}, err
	}
	var out api.Tenant
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		before, err := q.GetTenant(ctx, p.TenantID)
		if err != nil {
			return err
		}
		row, err := q.RenameTenant(ctx, name)
		if err != nil {
			return err
		}
		out = api.Tenant{Id: row.ID, Name: row.Name, CareersSlug: row.CareersSlug}
		if before.Name == name {
			return nil
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "tenant.renamed", EntityType: "tenant", EntityID: row.ID,
			Changes: map[string]any{"name": map[string]any{"from": before.Name, "to": name}}})
	})
	return out, err
}

// RenameAgency handles PUT /v1/agency.
func (h *HTTP) RenameAgency(ctx context.Context, req api.RenameAgencyRequestObject) (api.RenameAgencyResponseObject, error) {
	t, err := h.m.RenameAgency(ctx, authz.MustFromContext(ctx), req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.RenameAgency200JSONResponse(t), nil
}
