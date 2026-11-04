package identity

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetAccessSettings handles GET /v1/access.
func (h *HTTP) GetAccessSettings(ctx context.Context, _ api.GetAccessSettingsRequestObject) (api.GetAccessSettingsResponseObject, error) {
	all, err := h.m.AccessSettings(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.GetAccessSettings200JSONResponse{Roles: make([]api.RoleAccess, len(all))}
	for i, a := range all {
		out.Roles[i] = accessToAPI(a)
	}
	for _, p := range authz.Catalog() {
		out.Permissions = append(out.Permissions, api.PermissionInfo{
			Permission: p.Permission, Module: p.Module, Description: p.Description, Sensitive: p.Sensitive, DefaultRoles: p.DefaultRoles,
		})
	}
	return out, nil
}

// SetRoleAccess handles PUT /v1/access/roles/{role}.
func (h *HTTP) SetRoleAccess(ctx context.Context, req api.SetRoleAccessRequestObject) (api.SetRoleAccessResponseObject, error) {
	b := req.Body
	a, err := h.m.SetAccess(ctx, authz.MustFromContext(ctx), Access{
		Role: string(req.Role), Permissions: b.Permissions, Scope: string(b.CandidateScope), JobCandidates: b.JobCandidates,
	}, b.Version)
	if err != nil {
		return nil, err
	}
	return api.SetRoleAccess200JSONResponse(accessToAPI(a)), nil
}

// ResetRoleAccess handles DELETE /v1/access/roles/{role}.
func (h *HTTP) ResetRoleAccess(ctx context.Context, req api.ResetRoleAccessRequestObject) (api.ResetRoleAccessResponseObject, error) {
	a, err := h.m.ResetAccess(ctx, authz.MustFromContext(ctx), string(req.Role))
	if err != nil {
		return nil, err
	}
	return api.ResetRoleAccess200JSONResponse(accessToAPI(a)), nil
}

// ListTeams handles GET /v1/teams.
func (h *HTTP) ListTeams(ctx context.Context, _ api.ListTeamsRequestObject) (api.ListTeamsResponseObject, error) {
	teams, err := h.m.Teams(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListTeams200JSONResponse{Items: make([]api.TeamGroup, len(teams))}
	for i, t := range teams {
		out.Items[i] = teamToAPI(t)
	}
	return out, nil
}

// CreateTeam handles POST /v1/teams.
func (h *HTTP) CreateTeam(ctx context.Context, req api.CreateTeamRequestObject) (api.CreateTeamResponseObject, error) {
	t, err := h.m.CreateTeam(ctx, authz.MustFromContext(ctx), req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.CreateTeam201JSONResponse(teamToAPI(t)), nil
}

// RenameTeam handles PATCH /v1/teams/{teamId}.
func (h *HTTP) RenameTeam(ctx context.Context, req api.RenameTeamRequestObject) (api.RenameTeamResponseObject, error) {
	t, err := h.m.RenameTeam(ctx, authz.MustFromContext(ctx), req.TeamId, req.Body.Version, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.RenameTeam200JSONResponse(teamToAPI(t)), nil
}

// DeleteTeam handles DELETE /v1/teams/{teamId}.
func (h *HTTP) DeleteTeam(ctx context.Context, req api.DeleteTeamRequestObject) (api.DeleteTeamResponseObject, error) {
	if err := h.m.DeleteTeam(ctx, authz.MustFromContext(ctx), req.TeamId); err != nil {
		return nil, err
	}
	return api.DeleteTeam204Response{}, nil
}

func accessToAPI(a Access) api.RoleAccess {
	perms := a.Permissions
	if perms == nil {
		perms = []api.Permission{}
	}
	return api.RoleAccess{
		Role: api.Role(a.Role), Permissions: perms, CandidateScope: api.CandidateScope(a.Scope), JobCandidates: a.JobCandidates,
		Custom: a.Custom, Editable: Editable(a.Role), Version: a.Version,
	}
}

func teamToAPI(t TeamGroup) api.TeamGroup {
	return api.TeamGroup{Id: t.ID, Version: t.Version, Name: t.Name, MemberCount: t.MemberCount}
}
