package emails

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetMailApps handles GET /v1/settings/email-apps.
func (h *HTTP) GetMailApps(ctx context.Context, _ api.GetMailAppsRequestObject) (api.GetMailAppsResponseObject, error) {
	apps, err := h.svc.MailApps(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetMailApps200JSONResponse(mailAppsToAPI(apps)), nil
}

// SaveMailApp handles PUT /v1/settings/email-apps/{provider}.
func (h *HTTP) SaveMailApp(ctx context.Context, req api.SaveMailAppRequestObject) (api.SaveMailAppResponseObject, error) {
	apps, err := h.svc.SaveMailApp(ctx, authz.MustFromContext(ctx), string(req.Provider), req.Body.ClientId, req.Body.ClientSecret)
	if err != nil {
		return nil, err
	}
	return api.SaveMailApp200JSONResponse(mailAppsToAPI(apps)), nil
}

// DeleteMailApp handles DELETE /v1/settings/email-apps/{provider}.
func (h *HTTP) DeleteMailApp(ctx context.Context, req api.DeleteMailAppRequestObject) (api.DeleteMailAppResponseObject, error) {
	apps, err := h.svc.DeleteMailApp(ctx, authz.MustFromContext(ctx), string(req.Provider))
	if err != nil {
		return nil, err
	}
	return api.DeleteMailApp200JSONResponse(mailAppsToAPI(apps)), nil
}

func mailAppsToAPI(a MailApps) api.MailApps {
	app := func(m MailApp) api.MailApp {
		return api.MailApp{Source: api.MailAppSource(m.Source), ClientId: c.OrNull(m.ClientID), SecretEnding: c.OrNull(m.SecretEnding), UpdatedAt: c.OrNull(m.UpdatedAt)}
	}
	return api.MailApps{StorageReady: a.StorageReady, CallbackUrl: a.CallbackURL, Microsoft: app(a.Microsoft), Google: app(a.Google)}
}
