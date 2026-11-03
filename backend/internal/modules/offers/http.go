package offers

import (
	"context"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the offers operations of the contract.
type HTTP struct{ svc *Service }

// ListOffers handles GET /v1/offers.
func (h *HTTP) ListOffers(ctx context.Context, req api.ListOffersRequestObject) (api.ListOffersResponseObject, error) {
	views, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.Params.ApplicationId, c.Convert[string](req.Params.Status))
	if err != nil {
		return nil, err
	}
	out := api.ListOffers200JSONResponse{Items: make([]api.Offer, len(views))}
	for i, v := range views {
		out.Items[i] = toAPI(v)
	}
	return out, nil
}

// CreateOffer handles POST /v1/applications/{applicationId}/offers.
func (h *HTTP) CreateOffer(ctx context.Context, req api.CreateOfferRequestObject) (api.CreateOfferResponseObject, error) {
	b := req.Body
	terms := TermsPatch{
		StartDate: c.Change[*time.Time]{Set: true, Value: dayPtr(b.StartDate)},
		ExpiresOn: c.Change[*time.Time]{Set: true, Value: dayPtr(b.ExpiresOn)},
		Terms:     c.Change[*string]{Set: true, Value: b.Terms},
	}
	var salary *Salary
	if b.Salary != nil {
		salary = salaryIn(*b.Salary)
	}
	v, err := h.svc.Create(ctx, authz.MustFromContext(ctx), req.ApplicationId, terms, salary)
	if err != nil {
		return nil, err
	}
	return api.CreateOffer201JSONResponse(toAPI(v)), nil
}

// UpdateOffer handles PATCH /v1/offers/{offerId}.
func (h *HTTP) UpdateOffer(ctx context.Context, req api.UpdateOfferRequestObject) (api.UpdateOfferResponseObject, error) {
	b := req.Body
	terms := TermsPatch{
		StartDate: c.Optional(b.StartDate, dayOf),
		ExpiresOn: c.Optional(b.ExpiresOn, dayOf),
		Terms:     c.Optional(b.Terms, c.Same[string]),
	}
	var salary c.Change[*Salary]
	if b.Salary.IsSpecified() {
		salary.Set = true
		if s, err := b.Salary.Get(); err == nil {
			salary.Value = salaryIn(s)
		}
	}
	v, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.OfferId, b.Version, terms, salary)
	if err != nil {
		return nil, err
	}
	return api.UpdateOffer200JSONResponse(toAPI(v)), nil
}

// StepOffer handles POST /v1/offers/{offerId}/steps.
func (h *HTTP) StepOffer(ctx context.Context, req api.StepOfferRequestObject) (api.StepOfferResponseObject, error) {
	v, err := h.svc.Step(ctx, authz.MustFromContext(ctx), req.OfferId, req.Body.Version, string(req.Body.Step))
	if err != nil {
		return nil, err
	}
	return api.StepOffer200JSONResponse(toAPI(v)), nil
}

// DecideOffer handles POST /v1/offers/{offerId}/decision.
func (h *HTTP) DecideOffer(ctx context.Context, req api.DecideOfferRequestObject) (api.DecideOfferResponseObject, error) {
	b := req.Body
	v, err := h.svc.Decide(ctx, authz.MustFromContext(ctx), req.OfferId, b.Version, b.Approve, b.Note)
	if err != nil {
		return nil, err
	}
	return api.DecideOffer200JSONResponse(toAPI(v)), nil
}

func dayOf(d openapi_types.Date) time.Time { return d.Time }

func dayPtr(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	t := d.Time
	return &t
}

func dateOf(t *time.Time) nullable.Nullable[openapi_types.Date] {
	if t == nil {
		return nullable.NewNullNullable[openapi_types.Date]()
	}
	return nullable.NewNullableWithValue(openapi_types.Date{Time: *t})
}

func salaryIn(s api.SalaryAmount) *Salary {
	return &Salary{Amount: s.Amount, Currency: s.Currency, Period: string(s.Period)}
}

func toAPI(v View) api.Offer {
	o := v.Offer
	name := func(id *string) nullable.Nullable[string] {
		if id == nil {
			return nullable.NewNullNullable[string]()
		}
		if n, ok := v.Names[*id]; ok {
			return nullable.NewNullableWithValue(n)
		}
		return nullable.NewNullNullable[string]()
	}
	out := api.Offer{
		Id: v.ID, Version: v.Version, ApplicationId: v.ApplicationID,
		CandidateId: v.App.CandidateID, CandidateName: v.App.CandidateName, JobId: v.App.JobID, JobTitle: v.App.JobTitle, CompanyName: v.App.CompanyName,
		Status: api.OfferStatus(o.Status), StartDate: dateOf(o.StartDate), ExpiresOn: dateOf(o.ExpiresOn), Terms: c.OrNull(o.Terms),
		RequestedBy: c.OrNull(o.RequestedBy), RequestedByName: name(o.RequestedBy),
		DecidedBy: c.OrNull(o.DecidedBy), DecidedByName: name(o.DecidedBy), DecidedAt: c.OrNull(o.DecidedAt), DecisionNote: c.OrNull(o.DecisionNote),
		SentAt: c.OrNull(o.SentAt), AnsweredAt: c.OrNull(o.AnsweredAt),
		RestrictedFields: []api.OfferRestrictedField{}, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	switch {
	case v.SalaryHidden:
		out.RestrictedFields = append(out.RestrictedFields, api.OfferRestrictedFieldSalary)
	case v.Salary == nil:
		out.Salary = nullable.NewNullNullable[api.SalaryAmount]()
	default:
		out.Salary = nullable.NewNullableWithValue(api.SalaryAmount{Amount: v.Salary.Amount, Currency: v.Salary.Currency, Period: api.PayPeriod(v.Salary.Period)})
	}
	return out
}
