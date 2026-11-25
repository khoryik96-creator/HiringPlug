package identity

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
)

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// StartMicrosoftSignIn handles POST /v1/auth/microsoft/start.
func (h *HTTP) StartMicrosoftSignIn(ctx context.Context, req api.StartMicrosoftSignInRequestObject) (api.StartMicrosoftSignInResponseObject, error) {
	b := req.Body
	email := ""
	if b.Email != nil {
		email = string(*b.Email)
	}
	target, err := h.m.StartMicrosoft(ctx, clientip.From(ctx), MicrosoftStart{BindingHash: b.BindingHash, Email: email, Company: orEmpty(b.Company), Link: orEmpty(b.Invite)})
	if err != nil {
		return nil, err
	}
	return api.StartMicrosoftSignIn200JSONResponse{Url: target}, nil
}

// MicrosoftSignInCallback handles GET /v1/auth/microsoft/callback: never
// cached, and the address (with its code and state) never sent on as a referrer.
func (h *HTTP) MicrosoftSignInCallback(ctx context.Context, req api.MicrosoftSignInCallbackRequestObject) (api.MicrosoftSignInCallbackResponseObject, error) {
	p := req.Params
	target := h.m.MicrosoftCallback(ctx, clientip.From(ctx), orEmpty(p.State), orEmpty(p.Code), orEmpty(p.Error))
	cache, policy := "no-store", "no-referrer"
	return api.MicrosoftSignInCallback303Response{Headers: api.MicrosoftSignInCallback303ResponseHeaders{Location: &target, CacheControl: &cache, ReferrerPolicy: &policy}}, nil
}

// CompleteMicrosoftSignIn handles POST /v1/auth/microsoft/complete.
func (h *HTTP) CompleteMicrosoftSignIn(ctx context.Context, req api.CompleteMicrosoftSignInRequestObject) (api.CompleteMicrosoftSignInResponseObject, error) {
	b := req.Body
	s, err := h.m.CompleteMicrosoft(ctx, clientip.From(ctx), MicrosoftCompletion{Ticket: b.Ticket, Binding: b.Binding, Company: orEmpty(b.Company)})
	if err != nil {
		return nil, err
	}
	return api.CompleteMicrosoftSignIn200JSONResponse{Token: s.Token, ExpiresAt: s.ExpiresAt}, nil
}

// StartConnectingMicrosoft handles POST /v1/me/microsoft/start.
func (h *HTTP) StartConnectingMicrosoft(ctx context.Context, req api.StartConnectingMicrosoftRequestObject) (api.StartConnectingMicrosoftResponseObject, error) {
	b := req.Body
	target, err := h.m.StartConnectingMicrosoft(ctx, authz.MustFromContext(ctx), clientip.From(ctx), ConnectStart{BindingHash: b.BindingHash, Password: b.Password, Code: b.Code})
	if err != nil {
		return nil, err
	}
	return api.StartConnectingMicrosoft200JSONResponse{Url: target}, nil
}

// FinishConnectingMicrosoft handles POST /v1/me/microsoft/complete.
func (h *HTTP) FinishConnectingMicrosoft(ctx context.Context, req api.FinishConnectingMicrosoftRequestObject) (api.FinishConnectingMicrosoftResponseObject, error) {
	if err := h.m.FinishConnectingMicrosoft(ctx, authz.MustFromContext(ctx), clientip.From(ctx), req.Body.Ticket, req.Body.Binding); err != nil {
		return nil, err
	}
	return api.FinishConnectingMicrosoft204Response{}, nil
}

// UnlinkMicrosoft handles DELETE /v1/team/{userId}/microsoft.
func (h *HTTP) UnlinkMicrosoft(ctx context.Context, req api.UnlinkMicrosoftRequestObject) (api.UnlinkMicrosoftResponseObject, error) {
	if err := h.m.UnlinkMicrosoft(ctx, authz.MustFromContext(ctx), req.UserId); err != nil {
		return nil, err
	}
	return api.UnlinkMicrosoft204Response{}, nil
}

// SendMicrosoftReconnectLink handles POST /v1/team/{userId}/microsoft/reconnect.
func (h *HTTP) SendMicrosoftReconnectLink(ctx context.Context, req api.SendMicrosoftReconnectLinkRequestObject) (api.SendMicrosoftReconnectLinkResponseObject, error) {
	l, err := h.m.SendMicrosoftReconnectLink(ctx, authz.MustFromContext(ctx), req.UserId)
	if err != nil {
		return nil, err
	}
	return api.SendMicrosoftReconnectLink201JSONResponse{Link: l.Link, ExpiresAt: l.ExpiresAt, Emailed: l.Emailed}, nil
}

func microsoftSignInToAPI(s MicrosoftSignIn) api.MicrosoftSignInSettings {
	return api.MicrosoftSignInSettings{Enabled: s.Enabled, OrganisationId: c.OrNull(s.OrganisationID), Required: s.Required,
		CallbackUrl: s.CallbackURL, AppSource: api.MicrosoftSignInSettingsAppSource(s.AppSource), ClientId: c.OrNull(s.ClientID),
		AcctClaimMissing: s.AcctMissing}
}

// GetMicrosoftSignIn handles GET /v1/agency/microsoft-sign-in.
func (h *HTTP) GetMicrosoftSignIn(ctx context.Context, _ api.GetMicrosoftSignInRequestObject) (api.GetMicrosoftSignInResponseObject, error) {
	s, err := h.m.MicrosoftSignInSettings(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetMicrosoftSignIn200JSONResponse(microsoftSignInToAPI(s)), nil
}

// SetMicrosoftSignIn handles PUT /v1/agency/microsoft-sign-in.
func (h *HTTP) SetMicrosoftSignIn(ctx context.Context, req api.SetMicrosoftSignInRequestObject) (api.SetMicrosoftSignInResponseObject, error) {
	b := req.Body
	s, err := h.m.SetMicrosoftSignIn(ctx, authz.MustFromContext(ctx), b.Enabled, c.ValueOrNil(b.OrganisationId), b.Required)
	if err != nil {
		return nil, err
	}
	return api.SetMicrosoftSignIn200JSONResponse(microsoftSignInToAPI(s)), nil
}
