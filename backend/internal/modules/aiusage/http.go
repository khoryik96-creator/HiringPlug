package aiusage

// HTTP handlers: contract types <-> service calls; no business rules.

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the module's endpoints.
type HTTP struct{ svc *Service }

func featuresToAPI(fs []FeatureUse, p Prices) []api.AiUsageFeature {
	out := make([]api.AiUsageFeature, len(fs))
	for i, f := range fs {
		out[i] = api.AiUsageFeature{Feature: api.AiFeature(f.Feature), Uses: f.Uses, InputTokens: f.Spent.Input,
			OutputTokens: f.Spent.Output, Cost: c.OrNull(Cost(p, f.Spent))}
	}
	return out
}

func limitToAPI(l Limit, currency string, st Status) api.AiUsageLimit {
	out := api.AiUsageLimit{Kind: api.AiUsageLimitKind(l.Kind), Tokens: c.OrNull[int64](nil), Money: c.OrNull[string](nil), Currency: currency,
		WarnPercent: l.WarnPercent, UsedPercent: int(st.UsedPercent), //nolint:gosec // capped by UsedPercent
		Reached: st.Reached, ResetsAt: st.ResetsAt}
	if l.Kind == KindMoney {
		out.Money = c.OrNull(text(&l.Money, AmountPlaces))
	} else {
		out.Tokens = c.OrNull(&l.Tokens)
	}
	return out
}

// ReportToAPI is a report as the contract has it.
func ReportToAPI(r Report) api.AiUsage {
	p := r.Limit.Prices
	months := make([]api.AiUsageMonth, len(r.Months))
	for i, m := range r.Months {
		months[i] = api.AiUsageMonth{Month: m.Month, Uses: m.Uses, InputTokens: m.Spent.Input, OutputTokens: m.Spent.Output,
			Cost: c.OrNull(Cost(p, m.Spent)), ByFeature: featuresToAPI(m.Features, p)}
	}
	people := make([]api.AiUsagePerson, len(r.People))
	for i, u := range r.People {
		people[i] = api.AiUsagePerson{UserId: c.OrNull(u.UserID), Name: u.Name, Uses: u.Uses, Tokens: u.Spent.Tokens(), Cost: c.OrNull(Cost(p, u.Spent))}
	}
	this := months[0]
	return api.AiUsage{Month: this.Month, Uses: this.Uses, InputTokens: this.InputTokens, OutputTokens: this.OutputTokens, Cost: this.Cost,
		ByFeature: this.ByFeature, ByPerson: people, Months: months, Limit: limitToAPI(r.Limit, r.Currency, r.Status), PricesSet: p.Set}
}

// GetAiUsage handles GET /v1/ai-usage.
func (h *HTTP) GetAiUsage(ctx context.Context, req api.GetAiUsageRequestObject) (api.GetAiUsageResponseObject, error) {
	months := DefaultReportMonths
	if req.Params.Months != nil {
		months = *req.Params.Months
	}
	r, err := h.svc.Usage(ctx, authz.MustFromContext(ctx), months)
	if err != nil {
		return nil, err
	}
	return api.GetAiUsage200JSONResponse(ReportToAPI(r)), nil
}

func settingsToAPI(v SettingsView) api.AiUsageSettings {
	return api.AiUsageSettings{
		LimitKind: api.AiUsageLimitKind(v.Kind), LimitTokens: c.OrNull(v.Tokens), ServerDefaultTokens: v.ServerDefaultTokens,
		LimitMoney: c.OrNull(text(v.Money, AmountPlaces)), Currency: v.Currency,
		PriceInputPerMillion: c.OrNull(text(v.PriceInput, PricePlaces)), PriceOutputPerMillion: c.OrNull(text(v.PriceOutput, PricePlaces)),
		WarnPercent: v.WarnPercent, DailyTokens: c.OrNull(v.Daily), FeatureTokens: featureMap(v.PerFeature),
		Version: v.Version, UpdatedAt: c.OrNull(v.UpdatedAt),
	}
}

// GetAiUsageSettings handles GET /v1/ai-usage/settings.
func (h *HTTP) GetAiUsageSettings(ctx context.Context, _ api.GetAiUsageSettingsRequestObject) (api.GetAiUsageSettingsResponseObject, error) {
	v, err := h.svc.Settings(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetAiUsageSettings200JSONResponse(settingsToAPI(v)), nil
}

// PutAiUsageSettings handles PUT /v1/ai-usage/settings.
func (h *HTTP) PutAiUsageSettings(ctx context.Context, req api.PutAiUsageSettingsRequestObject) (api.PutAiUsageSettingsResponseObject, error) {
	b := req.Body
	v, err := h.svc.SaveSettings(ctx, authz.MustFromContext(ctx), SettingsInput{
		LimitKind: string(b.LimitKind), LimitTokens: c.ValueOrNil(b.LimitTokens), LimitMoney: c.ValueOrNil(b.LimitMoney), Currency: b.Currency,
		PriceInputPerMillion: c.ValueOrNil(b.PriceInputPerMillion), PriceOutputPerMillion: c.ValueOrNil(b.PriceOutputPerMillion),
		WarnPercent: b.WarnPercent, Limits: limitsFrom(b),
	}, b.Version)
	if err != nil {
		return nil, err
	}
	return api.PutAiUsageSettings200JSONResponse(settingsToAPI(v)), nil
}

// GetAiUsageStatus handles GET /v1/ai-usage/status.
func (h *HTTP) GetAiUsageStatus(ctx context.Context, _ api.GetAiUsageStatusRequestObject) (api.GetAiUsageStatusResponseObject, error) {
	st, err := h.svc.UsageStatus(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetAiUsageStatus200JSONResponse(api.AiUsageStatus{
		UsedPercent: int(st.UsedPercent), Warn: st.Warn, //nolint:gosec // capped by UsedPercent
		Reached: st.Reached, ResetsAt: st.ResetsAt,
	}), nil
}

// limitsFrom is the daily and per-feature limits sent (nil: neither was, so
// both are kept as they are; one left out keeps its own value).
func limitsFrom(b *api.PutAiUsageSettingsJSONRequestBody) *Limits {
	if !b.DailyTokens.IsSpecified() && b.FeatureTokens == nil {
		return nil
	}
	l := &Limits{Daily: c.ValueOrNil(b.DailyTokens), KeepDaily: !b.DailyTokens.IsSpecified(), KeepPerFeature: b.FeatureTokens == nil}
	if b.FeatureTokens != nil {
		l.PerFeature = *b.FeatureTokens
	}
	return l
}
