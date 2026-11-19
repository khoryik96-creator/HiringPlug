package emails

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

func (h *HTTP) ListOutlookRules(ctx context.Context, _ api.ListOutlookRulesRequestObject) (api.ListOutlookRulesResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.listOutlookRules(ctx, p)
	if err != nil {
		return nil, err
	}
	return api.ListOutlookRules200JSONResponse{
		Body: out, Headers: api.ListOutlookRules200ResponseHeaders{CacheControl: "no-store"},
	}, nil
}

func (h *HTTP) CreateOutlookRule(ctx context.Context, r api.CreateOutlookRuleRequestObject) (api.CreateOutlookRuleResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.createOutlookRule(ctx, p, *r.Body)
	return api.CreateOutlookRule201JSONResponse(out), err
}

func (h *HTTP) SetOutlookRuleEnabled(ctx context.Context, r api.SetOutlookRuleEnabledRequestObject) (api.SetOutlookRuleEnabledResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.SetOutlookRuleEnabled204Response{}, h.svc.changeOutlookRule(ctx, p, r.RuleId, &r.Body.Enabled)
}

func (h *HTTP) DeleteOutlookRule(ctx context.Context, r api.DeleteOutlookRuleRequestObject) (api.DeleteOutlookRuleResponseObject, error) {
	p, err := mailboxUser(ctx)
	if err != nil {
		return nil, err
	}
	return api.DeleteOutlookRule204Response{}, h.svc.changeOutlookRule(ctx, p, r.RuleId, nil)
}
