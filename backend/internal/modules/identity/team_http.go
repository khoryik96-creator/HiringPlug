package identity

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
)

// SignIn handles POST /v1/auth/sign-in.
func (h *HTTP) SignIn(ctx context.Context, req api.SignInRequestObject) (api.SignInResponseObject, error) {
	b := req.Body
	company := ""
	if b.Company != nil {
		company = *b.Company
	}
	s, err := h.m.SignIn(ctx, clientip.From(ctx), string(b.Email), b.Password, company)
	if err != nil {
		return nil, err
	}
	return api.SignIn200JSONResponse{Token: s.Token, ExpiresAt: s.ExpiresAt}, nil
}

// RequestPasswordReset handles POST /v1/auth/password-resets.
func (h *HTTP) RequestPasswordReset(ctx context.Context, req api.RequestPasswordResetRequestObject) (api.RequestPasswordResetResponseObject, error) {
	if err := h.m.RequestReset(ctx, clientip.From(ctx), string(req.Body.Email)); err != nil {
		return nil, err
	}
	return api.RequestPasswordReset202Response{}, nil
}

// GetAuthLink handles GET /v1/auth/links/{linkToken}.
func (h *HTTP) GetAuthLink(ctx context.Context, req api.GetAuthLinkRequestObject) (api.GetAuthLinkResponseObject, error) {
	l, err := h.m.Link(ctx, clientip.From(ctx), req.LinkToken)
	if err != nil {
		return nil, err
	}
	return api.GetAuthLink200JSONResponse{Kind: api.AuthLinkKind(l.Kind), Email: l.Email, Name: l.Name, CompanyName: l.CompanyName}, nil
}

// UseAuthLink handles POST /v1/auth/links/{linkToken}.
func (h *HTTP) UseAuthLink(ctx context.Context, req api.UseAuthLinkRequestObject) (api.UseAuthLinkResponseObject, error) {
	s, err := h.m.UseLink(ctx, clientip.From(ctx), req.LinkToken, req.Body.Password, req.Body.Name)
	if err != nil {
		return nil, err
	}
	return api.UseAuthLink200JSONResponse{Token: s.Token, ExpiresAt: s.ExpiresAt}, nil
}

// ChangePassword handles PUT /v1/me/password.
func (h *HTTP) ChangePassword(ctx context.Context, req api.ChangePasswordRequestObject) (api.ChangePasswordResponseObject, error) {
	s, err := h.m.ChangePassword(ctx, authz.MustFromContext(ctx), req.Body.CurrentPassword, req.Body.NewPassword)
	if err != nil {
		return nil, err
	}
	return api.ChangePassword200JSONResponse{Token: s.Token, ExpiresAt: s.ExpiresAt}, nil
}

// ListTeam handles GET /v1/team.
func (h *HTTP) ListTeam(ctx context.Context, _ api.ListTeamRequestObject) (api.ListTeamResponseObject, error) {
	rows, err := h.m.Team(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListTeam200JSONResponse{Items: make([]api.TeamMember, len(rows))}
	for i, r := range rows {
		out.Items[i] = memberToAPI(r)
	}
	return out, nil
}

// InviteUser handles POST /v1/team/invites.
func (h *HTTP) InviteUser(ctx context.Context, req api.InviteUserRequestObject) (api.InviteUserResponseObject, error) {
	b := req.Body
	inv, err := h.m.Invite(ctx, authz.MustFromContext(ctx), string(b.Email), b.Name, string(b.Role))
	if err != nil {
		return nil, err
	}
	return api.InviteUser201JSONResponse(invitationToAPI(inv)), nil
}

// ResendInvite handles POST /v1/team/{userId}/invite.
func (h *HTTP) ResendInvite(ctx context.Context, req api.ResendInviteRequestObject) (api.ResendInviteResponseObject, error) {
	inv, err := h.m.ResendInvite(ctx, authz.MustFromContext(ctx), req.UserId)
	if err != nil {
		return nil, err
	}
	return api.ResendInvite201JSONResponse(invitationToAPI(inv)), nil
}

// UpdateTeamMember handles PATCH /v1/team/{userId}.
func (h *HTTP) UpdateTeamMember(ctx context.Context, req api.UpdateTeamMemberRequestObject) (api.UpdateTeamMemberResponseObject, error) {
	b := req.Body
	var team **string
	if b.TeamId.IsSpecified() {
		t := c.ValueOrNil(b.TeamId)
		team = &t
	}
	row, err := h.m.UpdateMember(ctx, authz.MustFromContext(ctx), req.UserId, c.Convert[string](b.Role), b.Active, team)
	if err != nil {
		return nil, err
	}
	return api.UpdateTeamMember200JSONResponse(memberToAPI(row)), nil
}

func memberToAPI(r TeamMember) api.TeamMember {
	return api.TeamMember{
		Id: r.ID, Name: r.Name, Email: r.Email, Role: api.Role(r.Role), TeamId: c.OrNull(r.TeamID), Status: api.TeamStatus(Status(r)),
		LastSignInAt: c.OrNull(r.LastSignInAt), CreatedAt: r.CreatedAt,
	}
}

func invitationToAPI(inv Invitation) api.Invitation {
	return api.Invitation{Member: memberToAPI(inv.Member), Link: inv.Link, ExpiresAt: inv.ExpiresAt, Emailed: inv.Emailed}
}
