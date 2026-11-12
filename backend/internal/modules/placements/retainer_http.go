package placements

import (
	"context"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetJobRetainer handles GET /v1/jobs/{jobId}/retainer.
func (h *HTTP) GetJobRetainer(ctx context.Context, req api.GetJobRetainerRequestObject) (api.GetJobRetainerResponseObject, error) {
	v, err := h.svc.GetRetainer(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	return api.GetJobRetainer200JSONResponse(retainerToAPI(v)), nil
}

// PutJobRetainer handles PUT /v1/jobs/{jobId}/retainer.
func (h *HTTP) PutJobRetainer(ctx context.Context, req api.PutJobRetainerRequestObject) (api.PutJobRetainerResponseObject, error) {
	b := req.Body
	in := Retainer{Version: b.Version, Currency: deref(b.Currency), Stages: make([]RetainerStage, len(b.Stages))}
	if d, err := b.SignedOn.Get(); err == nil {
		in.SignedOn = &d.Time
	}
	for i, st := range b.Stages {
		in.Stages[i] = RetainerStage{
			Label: st.Label, Amount: st.Amount, DueKind: string(st.DueKind), DueOn: dayPtr(st.DueOn), DueDays: st.DueDays,
			InvoiceStatus: deref(c.Convert[string](st.InvoiceStatus)), InvoiceNumber: st.InvoiceNumber,
			InvoicedOn: dayPtr(st.InvoicedOn), PaidOn: dayPtr(st.PaidOn),
		}
	}
	v, err := h.svc.PutRetainer(ctx, authz.MustFromContext(ctx), req.JobId, in)
	if err != nil {
		return nil, err
	}
	return api.PutJobRetainer200JSONResponse(retainerToAPI(v)), nil
}

func retainerToAPI(v RetainerView) api.JobRetainer {
	out := api.JobRetainer{Version: v.Version, SignedOn: dateOf(v.SignedOn), Stages: stagesToAPI(v), Total: v.Total(), PlacementId: c.OrNull(v.PlacementID)}
	out.Currency = nullable.NewNullNullable[string]()
	if v.Version > 0 {
		out.Currency = nullable.NewNullableWithValue(v.Currency)
	}
	return out
}

func stagesToAPI(v RetainerView) []api.RetainerStage {
	out := make([]api.RetainerStage, len(v.Stages))
	for i, st := range v.Stages {
		out[i] = api.RetainerStage{
			Label: st.Label, Amount: st.Amount, DueKind: api.RetainerDueKind(st.DueKind), DueOn: dateOf(v.DueDay(st, v.Start)),
			DueDays: c.OrNull(st.DueDays), InvoiceStatus: api.InvoiceStatus(st.InvoiceStatus), InvoiceNumber: c.OrNull(st.InvoiceNumber),
			InvoicedOn: dateOf(st.InvoicedOn), PaidOn: dateOf(st.PaidOn),
		}
	}
	return out
}

// placementRetainerToAPI is the retainer taken off a placement's fee, and the balance.
func placementRetainerToAPI(v *RetainerView, m Money) nullable.Nullable[api.PlacementRetainer] {
	if v == nil {
		return nullable.NewNullNullable[api.PlacementRetainer]()
	}
	out := api.PlacementRetainer{Currency: v.Currency, Total: v.Total(), Stages: stagesToAPI(*v), Balance: nullable.NewNullNullable[string]()}
	if m.FeeAmount != nil && m.FeeCurrency != nil {
		if balance, ok := v.Balance(*m.FeeAmount, *m.FeeCurrency); ok {
			out.Balance = nullable.NewNullableWithValue(balance)
		}
	}
	return nullable.NewNullableWithValue(out)
}
