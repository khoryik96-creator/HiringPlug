package identity

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/clientip"
)

// SignInTwoStep handles POST /v1/auth/two-step.
func (h *HTTP) SignInTwoStep(ctx context.Context, req api.SignInTwoStepRequestObject) (api.SignInTwoStepResponseObject, error) {
	s, err := h.m.SignInTwoStep(ctx, clientip.From(ctx), req.Body.Challenge, req.Body.Code)
	if err != nil {
		return nil, err
	}
	return api.SignInTwoStep200JSONResponse{Token: s.Token, ExpiresAt: s.ExpiresAt}, nil
}

// StartTwoStepAtSignIn handles POST /v1/auth/two-step/setup.
func (h *HTTP) StartTwoStepAtSignIn(ctx context.Context, req api.StartTwoStepAtSignInRequestObject) (api.StartTwoStepAtSignInResponseObject, error) {
	s, err := h.m.StartTwoStepAtSignIn(ctx, clientip.From(ctx), req.Body.Challenge)
	if err != nil {
		return nil, err
	}
	return api.StartTwoStepAtSignIn200JSONResponse{Secret: s.Secret, Uri: s.URI}, nil
}

// ConfirmTwoStepAtSignIn handles POST /v1/auth/two-step/setup/confirm.
func (h *HTTP) ConfirmTwoStepAtSignIn(ctx context.Context, req api.ConfirmTwoStepAtSignInRequestObject) (api.ConfirmTwoStepAtSignInResponseObject, error) {
	s, codes, err := h.m.ConfirmTwoStepAtSignIn(ctx, clientip.From(ctx), req.Body.Challenge, req.Body.Code)
	if err != nil {
		return nil, err
	}
	return api.ConfirmTwoStepAtSignIn200JSONResponse{Session: api.Session{Token: s.Token, ExpiresAt: s.ExpiresAt}, RecoveryCodes: codes}, nil
}

func statusToAPI(s TwoStepStatus) api.TwoStepStatus {
	out := api.TwoStepStatus{Enabled: s.Enabled, RecoveryCodesLeft: s.RecoveryCodesLeft, Required: s.Required}
	out.EnabledAt.SetNull()
	if s.EnabledAt != nil {
		out.EnabledAt.Set(*s.EnabledAt)
	}
	return out
}

// GetMyTwoStep handles GET /v1/me/two-step.
func (h *HTTP) GetMyTwoStep(ctx context.Context, _ api.GetMyTwoStepRequestObject) (api.GetMyTwoStepResponseObject, error) {
	s, err := h.m.MyTwoStep(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetMyTwoStep200JSONResponse(statusToAPI(s)), nil
}

// StartMyTwoStep handles POST /v1/me/two-step/setup.
func (h *HTTP) StartMyTwoStep(ctx context.Context, _ api.StartMyTwoStepRequestObject) (api.StartMyTwoStepResponseObject, error) {
	s, err := h.m.StartMyTwoStep(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.StartMyTwoStep200JSONResponse{Secret: s.Secret, Uri: s.URI}, nil
}

// ConfirmMyTwoStep handles POST /v1/me/two-step/confirm.
func (h *HTTP) ConfirmMyTwoStep(ctx context.Context, req api.ConfirmMyTwoStepRequestObject) (api.ConfirmMyTwoStepResponseObject, error) {
	codes, err := h.m.ConfirmMyTwoStep(ctx, authz.MustFromContext(ctx), req.Body.Code)
	if err != nil {
		return nil, err
	}
	return api.ConfirmMyTwoStep200JSONResponse{RecoveryCodes: codes}, nil
}

// NewRecoveryCodes handles POST /v1/me/two-step/recovery-codes.
func (h *HTTP) NewRecoveryCodes(ctx context.Context, req api.NewRecoveryCodesRequestObject) (api.NewRecoveryCodesResponseObject, error) {
	codes, err := h.m.NewRecoveryCodes(ctx, authz.MustFromContext(ctx), req.Body.Code)
	if err != nil {
		return nil, err
	}
	return api.NewRecoveryCodes200JSONResponse{RecoveryCodes: codes}, nil
}

// DisableMyTwoStep handles POST /v1/me/two-step/disable.
func (h *HTTP) DisableMyTwoStep(ctx context.Context, req api.DisableMyTwoStepRequestObject) (api.DisableMyTwoStepResponseObject, error) {
	s, err := h.m.DisableMyTwoStep(ctx, authz.MustFromContext(ctx), req.Body.Password)
	if err != nil {
		return nil, err
	}
	return api.DisableMyTwoStep200JSONResponse(statusToAPI(s)), nil
}

// ResetTwoStep handles DELETE /v1/team/{userId}/two-step.
func (h *HTTP) ResetTwoStep(ctx context.Context, req api.ResetTwoStepRequestObject) (api.ResetTwoStepResponseObject, error) {
	if err := h.m.ResetTwoStep(ctx, authz.MustFromContext(ctx), req.UserId); err != nil {
		return nil, err
	}
	return api.ResetTwoStep204Response{}, nil
}

// SetTwoStepRequired handles PUT /v1/agency/two-step.
func (h *HTTP) SetTwoStepRequired(ctx context.Context, req api.SetTwoStepRequiredRequestObject) (api.SetTwoStepRequiredResponseObject, error) {
	if err := h.m.SetTwoStepRequired(ctx, authz.MustFromContext(ctx), req.Body.Required); err != nil {
		return nil, err
	}
	return api.SetTwoStepRequired200JSONResponse{Required: req.Body.Required}, nil
}
