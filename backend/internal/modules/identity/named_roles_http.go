package identity

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func namedRoleToAPI(r NamedRole) api.NamedRole {
	perms := r.Access.Permissions
	if perms == nil {
		perms = []api.Permission{}
	}
	return api.NamedRole{Id: r.ID, Name: r.Name, BaseRole: api.Role(r.BaseRole), Permissions: perms,
		CandidateScope: api.CandidateScope(r.Access.Scope), JobCandidates: r.Access.JobCandidates, Members: r.Members, Version: r.Version}
}

// ListNamedRoles handles GET /v1/access/named-roles.
func (h *HTTP) ListNamedRoles(ctx context.Context, _ api.ListNamedRolesRequestObject) (api.ListNamedRolesResponseObject, error) {
	roles, err := h.m.NamedRoles(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.NamedRoleList{Items: make([]api.NamedRole, len(roles))}
	for i, r := range roles {
		out.Items[i] = namedRoleToAPI(r)
	}
	return api.ListNamedRoles200JSONResponse(out), nil
}

// CreateNamedRole handles POST /v1/access/named-roles.
func (h *HTTP) CreateNamedRole(ctx context.Context, req api.CreateNamedRoleRequestObject) (api.CreateNamedRoleResponseObject, error) {
	r, err := h.m.CreateNamedRole(ctx, authz.MustFromContext(ctx), req.Body.Name, string(req.Body.BaseRole))
	if err != nil {
		return nil, err
	}
	return api.CreateNamedRole201JSONResponse(namedRoleToAPI(r)), nil
}

// UpdateNamedRole handles PUT /v1/access/named-roles/{namedRoleId}.
func (h *HTTP) UpdateNamedRole(ctx context.Context, req api.UpdateNamedRoleRequestObject) (api.UpdateNamedRoleResponseObject, error) {
	b := req.Body
	r, err := h.m.UpdateNamedRole(ctx, authz.MustFromContext(ctx), req.NamedRoleId, b.Version, b.Name,
		Access{Permissions: b.Permissions, Scope: string(b.CandidateScope), JobCandidates: b.JobCandidates})
	if err != nil {
		return nil, err
	}
	return api.UpdateNamedRole200JSONResponse(namedRoleToAPI(r)), nil
}

// DeleteNamedRole handles DELETE /v1/access/named-roles/{namedRoleId}.
func (h *HTTP) DeleteNamedRole(ctx context.Context, req api.DeleteNamedRoleRequestObject) (api.DeleteNamedRoleResponseObject, error) {
	if err := h.m.DeleteNamedRole(ctx, authz.MustFromContext(ctx), req.NamedRoleId); err != nil {
		return nil, err
	}
	return api.DeleteNamedRole204Response{}, nil
}
