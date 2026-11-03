package integrations

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the integrations operations of the contract.
type HTTP struct{ svc *Service }

// ListApiKeys handles GET /v1/api-keys.
func (h *HTTP) ListApiKeys(ctx context.Context, _ api.ListApiKeysRequestObject) (api.ListApiKeysResponseObject, error) { //nolint:revive // generated name
	keys, names, err := h.svc.APIKeys(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListApiKeys200JSONResponse{Items: make([]api.ApiKey, len(keys))}
	for i, k := range keys {
		out.Items[i] = keyToAPI(k, names)
	}
	return out, nil
}

// CreateApiKey handles POST /v1/api-keys.
func (h *HTTP) CreateApiKey(ctx context.Context, req api.CreateApiKeyRequestObject) (api.CreateApiKeyResponseObject, error) { //nolint:revive // generated name
	p := authz.MustFromContext(ctx)
	k, key, err := h.svc.CreateAPIKey(ctx, p, req.Body.Name, string(req.Body.Role))
	if err != nil {
		return nil, err
	}
	return api.CreateApiKey201JSONResponse{ApiKey: keyToAPI(k, map[string]string{p.UserID: p.Name}), Key: key}, nil
}

// RevokeApiKey handles DELETE /v1/api-keys/{apiKeyId}.
func (h *HTTP) RevokeApiKey(ctx context.Context, req api.RevokeApiKeyRequestObject) (api.RevokeApiKeyResponseObject, error) { //nolint:revive // generated name
	if err := h.svc.RevokeAPIKey(ctx, authz.MustFromContext(ctx), req.ApiKeyId); err != nil {
		return nil, err
	}
	return api.RevokeApiKey204Response{}, nil
}

// ListWebhooks handles GET /v1/webhooks.
func (h *HTTP) ListWebhooks(ctx context.Context, _ api.ListWebhooksRequestObject) (api.ListWebhooksResponseObject, error) {
	rows, err := h.svc.Webhooks(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListWebhooks200JSONResponse{Items: make([]api.Webhook, len(rows)), Events: events(Events())}
	for i, r := range rows {
		out.Items[i] = hookToAPI(r)
	}
	return out, nil
}

// CreateWebhook handles POST /v1/webhooks.
func (h *HTTP) CreateWebhook(ctx context.Context, req api.CreateWebhookRequestObject) (api.CreateWebhookResponseObject, error) {
	b := req.Body
	row, secret, err := h.svc.CreateWebhook(ctx, authz.MustFromContext(ctx), Hook{URL: b.Url, Events: eventNames(b.Events), Active: true, Description: b.Description})
	if err != nil {
		return nil, err
	}
	return api.CreateWebhook201JSONResponse{Webhook: hookToAPI(row), Secret: secret}, nil
}

// UpdateWebhook handles PATCH /v1/webhooks/{webhookId}.
func (h *HTTP) UpdateWebhook(ctx context.Context, req api.UpdateWebhookRequestObject) (api.UpdateWebhookResponseObject, error) {
	b := req.Body
	patch := HookPatch{URL: b.Url, Active: b.Active}
	if b.Events != nil {
		e := eventNames(*b.Events)
		patch.Events = &e
	}
	if d := c.Optional(b.Description, c.Same[string]); d.Set {
		patch.SetDesc, patch.Description = true, d.Value
	}
	row, err := h.svc.UpdateWebhook(ctx, authz.MustFromContext(ctx), req.WebhookId, b.Version, patch)
	if err != nil {
		return nil, err
	}
	return api.UpdateWebhook200JSONResponse(hookToAPI(row)), nil
}

// DeleteWebhook handles DELETE /v1/webhooks/{webhookId}.
func (h *HTTP) DeleteWebhook(ctx context.Context, req api.DeleteWebhookRequestObject) (api.DeleteWebhookResponseObject, error) {
	if err := h.svc.DeleteWebhook(ctx, authz.MustFromContext(ctx), req.WebhookId); err != nil {
		return nil, err
	}
	return api.DeleteWebhook204Response{}, nil
}

// ListWebhookDeliveries handles GET /v1/webhooks/{webhookId}/deliveries.
func (h *HTTP) ListWebhookDeliveries(ctx context.Context, req api.ListWebhookDeliveriesRequestObject) (api.ListWebhookDeliveriesResponseObject, error) {
	rows, err := h.svc.Deliveries(ctx, authz.MustFromContext(ctx), req.WebhookId)
	if err != nil {
		return nil, err
	}
	out := api.ListWebhookDeliveries200JSONResponse{Items: make([]api.WebhookDelivery, len(rows))}
	for i, r := range rows {
		out.Items[i] = deliveryToAPI(r)
	}
	return out, nil
}

// PingWebhook handles POST /v1/webhooks/{webhookId}/deliveries.
func (h *HTTP) PingWebhook(ctx context.Context, req api.PingWebhookRequestObject) (api.PingWebhookResponseObject, error) {
	row, err := h.svc.Ping(ctx, authz.MustFromContext(ctx), req.WebhookId)
	if err != nil {
		return nil, err
	}
	return api.PingWebhook202JSONResponse(deliveryToAPI(row)), nil
}

func eventNames(in []api.WebhookEvent) []string {
	out := make([]string, len(in))
	for i, e := range in {
		out[i] = string(e)
	}
	return out
}

func events(in []string) []api.WebhookEvent {
	out := make([]api.WebhookEvent, len(in))
	for i, e := range in {
		out[i] = api.WebhookEvent(e)
	}
	return out
}

func keyToAPI(k store.ApiKey, names map[string]string) api.ApiKey {
	var name *string
	if n, ok := names[k.CreatedBy]; ok {
		name = &n
	}
	return api.ApiKey{
		Id: k.ID, Name: k.Name, Role: api.ApiKeyRole(k.Role), Prefix: k.Prefix, CreatedAt: k.CreatedAt,
		CreatedByName: c.OrNull(name), LastUsedAt: c.OrNull(k.LastUsedAt),
	}
}

func hookToAPI(r store.Webhook) api.Webhook {
	return api.Webhook{
		Id: r.ID, Version: int(r.Version), Url: r.Url, Events: events(r.Events), Active: r.Active,
		Description: c.OrNull(r.Description), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func deliveryToAPI(r store.WebhookDelivery) api.WebhookDelivery {
	var status *int
	if r.ResponseStatus != nil {
		s := int(*r.ResponseStatus)
		status = &s
	}
	return api.WebhookDelivery{
		Id: r.ID, Event: api.WebhookEvent(r.Event), Status: api.WebhookDeliveryStatus(r.Status), Attempts: int(r.Attempts),
		ResponseStatus: c.OrNull(status), LastError: c.OrNull(r.LastError), CreatedAt: r.CreatedAt,
		DeliveredAt: c.OrNull(r.DeliveredAt), NextAttemptAt: c.OrNull(r.NextAttemptAt),
	}
}
