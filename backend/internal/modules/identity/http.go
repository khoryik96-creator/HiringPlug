package identity

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the identity operations of the contract.
type HTTP struct{ m *Module }

// GetMe returns the signed-in user, their tenant and their permissions, so
// the web app can show or hide features.
func (h *HTTP) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	p := authz.MustFromContext(ctx)
	tenantName, err := h.m.tenantName(ctx, p)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse{
		User:        api.User{Id: p.UserID, Email: openapi_types.Email(p.Email), Name: p.Name},
		Tenant:      api.Tenant{Id: p.TenantID, Name: tenantName},
		Role:        p.Role,
		Permissions: p.Permissions(),
	}, nil
}
