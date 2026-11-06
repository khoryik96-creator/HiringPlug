package aiproviders

import (
	"context"
	"time"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP adapts the service to the generated API interface.
type HTTP struct{ svc *Service }

// GetAiProviders handles GET /v1/ai-providers.
func (h *HTTP) GetAiProviders(ctx context.Context, _ api.GetAiProvidersRequestObject) (api.GetAiProvidersResponseObject, error) {
	st, err := h.svc.Get(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetAiProviders200JSONResponse(toAPI(st)), nil
}

// ChooseAiProvider handles PUT /v1/ai-providers.
func (h *HTTP) ChooseAiProvider(ctx context.Context, req api.ChooseAiProviderRequestObject) (api.ChooseAiProviderResponseObject, error) {
	st, err := h.svc.Choose(ctx, authz.MustFromContext(ctx), string(req.Body.Provider))
	if err != nil {
		return nil, err
	}
	return api.ChooseAiProvider200JSONResponse(toAPI(st)), nil
}

// SaveAiProvider handles PUT /v1/ai-providers/{aiProvider}.
func (h *HTTP) SaveAiProvider(ctx context.Context, req api.SaveAiProviderRequestObject) (api.SaveAiProviderResponseObject, error) {
	st, err := h.svc.Save(ctx, authz.MustFromContext(ctx), string(req.AiProvider), req.Body.Model, req.Body.ApiKey)
	if err != nil {
		return nil, err
	}
	return api.SaveAiProvider200JSONResponse(toAPI(st)), nil
}

// RemoveAiProviderKey handles DELETE /v1/ai-providers/{aiProvider}.
func (h *HTTP) RemoveAiProviderKey(ctx context.Context, req api.RemoveAiProviderKeyRequestObject) (api.RemoveAiProviderKeyResponseObject, error) {
	st, err := h.svc.Remove(ctx, authz.MustFromContext(ctx), string(req.AiProvider))
	if err != nil {
		return nil, err
	}
	return api.RemoveAiProviderKey200JSONResponse(toAPI(st)), nil
}

// CheckAiProvider handles POST /v1/ai-providers/{aiProvider}/check.
func (h *HTTP) CheckAiProvider(ctx context.Context, req api.CheckAiProviderRequestObject) (api.CheckAiProviderResponseObject, error) {
	st, err := h.svc.Check(ctx, authz.MustFromContext(ctx), string(req.AiProvider))
	if err != nil {
		return nil, err
	}
	return api.CheckAiProvider202JSONResponse(toAPI(st)), nil
}

func toAPI(st State) api.AiProviders {
	out := api.AiProviders{
		Active: api.AiProviderChoice(st.Active), ServerAvailable: st.ServerAvailable, KeysSupported: st.KeysSupported,
		Providers: make([]api.AiProviderEntry, len(Providers)),
	}
	for i, name := range Providers {
		e := api.AiProviderEntry{
			Provider: api.AiProviderName(name), Model: nullable.NewNullNullable[string](), KeyHint: nullable.NewNullNullable[string](),
			UpdatedAt: nullable.NewNullNullable[time.Time](), Check: nullable.NewNullNullable[api.AiProviderCheck](),
		}
		if s, ok := st.Saved[name]; ok {
			e.Model, e.KeyHint, e.UpdatedAt = nullable.NewNullableWithValue(s.Model), nullable.NewNullableWithValue(s.KeyHint), nullable.NewNullableWithValue(s.UpdatedAt)
			if s.CheckStatus != nil && s.CheckedAt != nil {
				c := api.AiProviderCheck{Status: api.AiProviderCheckStatus(*s.CheckStatus), CheckedAt: *s.CheckedAt, Message: nullable.NewNullNullable[string]()}
				if s.CheckMessage != nil {
					c.Message = nullable.NewNullableWithValue(*s.CheckMessage)
				}
				e.Check = nullable.NewNullableWithValue(c)
			}
		}
		out.Providers[i] = e
	}
	return out
}
