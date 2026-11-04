package identity

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// HTTP implements the identity operations of the contract.
type HTTP struct{ m *Module }

// GetMe returns the signed-in user, their tenant and their permissions, so
// the web app can show or hide features.
func (h *HTTP) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	p := authz.MustFromContext(ctx)
	tenant, err := h.m.tenant(ctx, p)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse{
		User:        api.User{Id: p.UserID, Email: openapi_types.Email(p.Email), Name: p.Name},
		Tenant:      api.Tenant{Id: p.TenantID, Name: tenant.Name, CareersSlug: tenant.CareersSlug},
		Role:        p.Role,
		Permissions: p.Permissions(),
	}, nil
}

// ListUsers handles GET /v1/users: the company's active users.
func (h *HTTP) ListUsers(ctx context.Context, _ api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	var rows []store.ListActiveUsersRow
	perms := map[string][]api.Permission{} // per role, as the company set it
	err := database.InTx(ctx, h.m.pool, authz.MustFromContext(ctx), func(tx pgx.Tx) error {
		q := store.New(tx)
		var err error
		if rows, err = q.ListActiveUsers(ctx); err != nil {
			return err
		}
		for _, r := range rows {
			if _, done := perms[r.Role]; !done {
				a, err := roleAccess(ctx, q, r.Role)
				if err != nil {
					return err
				}
				perms[r.Role] = a.Permissions
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := api.ListUsers200JSONResponse{Items: make([]api.UserSummary, len(rows))}
	for i, r := range rows {
		out.Items[i] = api.UserSummary{Id: r.ID, Name: r.Name, Role: api.Role(r.Role), CanInterview: slices.Contains(perms[r.Role], api.PermissionInterviewsFeedback)}
	}
	return out, nil
}
