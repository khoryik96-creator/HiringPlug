package placements

import (
	"context"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the placements operations of the contract.
type HTTP struct{ svc *Service }

// ListPlacements handles GET /v1/placements.
func (h *HTTP) ListPlacements(ctx context.Context, req api.ListPlacementsRequestObject) (api.ListPlacementsResponseObject, error) {
	q := req.Params
	listed, err := h.svc.List(ctx, authz.MustFromContext(ctx), Filter{
		ApplicationID: q.ApplicationId, Status: c.Convert[string](q.Status), InvoiceStatus: c.Convert[string](q.InvoiceStatus),
		StartFrom: dayPtr(q.StartFrom), StartTo: dayPtr(q.StartTo),
	})
	if err != nil {
		return nil, err
	}
	out := api.ListPlacements200JSONResponse{Items: make([]api.Placement, len(listed.Items)), Total: listed.Total,
		Today: openapi_types.Date{Time: h.svc.Today(time.Now())}}
	for i, v := range listed.Items {
		out.Items[i] = toAPI(v)
	}
	if authz.MustFromContext(ctx).Can(api.PermissionPlacementsFeesRead) {
		list := feeTotalsToAPI(listed.Totals)
		out.FeeTotals = &list
	}
	return out, nil
}

// GetCompanyBilling handles GET /v1/companies/{companyId}/billing.
func (h *HTTP) GetCompanyBilling(ctx context.Context, req api.GetCompanyBillingRequestObject) (api.GetCompanyBillingResponseObject, error) {
	b, err := h.svc.CompanyBilling(ctx, authz.MustFromContext(ctx), req.CompanyId, time.Now())
	if err != nil {
		return nil, err
	}
	return api.GetCompanyBilling200JSONResponse{Year: b.Year, ThisYear: feeTotalsToAPI(b.ThisYear), AllTime: feeTotalsToAPI(b.AllTime)}, nil
}

func feeTotalsToAPI(totals []FeeTotal) []api.FeeTotal {
	list := make([]api.FeeTotal, len(totals))
	for i, t := range totals {
		list[i] = api.FeeTotal{Currency: t.Currency, Total: t.Total, NotInvoiced: t.NotInvoiced, Invoiced: t.Invoiced, Paid: t.Paid}
	}
	return list
}

// CreatePlacement handles POST /v1/applications/{applicationId}/placement.
func (h *HTTP) CreatePlacement(ctx context.Context, req api.CreatePlacementRequestObject) (api.CreatePlacementResponseObject, error) {
	b := req.Body
	d := Details{StartDate: b.StartDate.Time, GuaranteeDays: DefaultGuaranteeDays, RecruiterID: b.RecruiterId, Sheet: Sheet{
		JobTitle: b.JobTitle, CompanyName: b.CompanyName, JobType: deref(c.Convert[string](b.JobType)), PONumber: b.PoNumber, EndDate: dayPtr(b.EndDate),
	}}
	if b.GuaranteeDays != nil {
		d.GuaranteeDays = *b.GuaranteeDays
	}
	var money *Money
	if b.Money != nil {
		money = moneyIn(*b.Money)
	}
	v, err := h.svc.Create(ctx, authz.MustFromContext(ctx), req.ApplicationId, d, money)
	if err != nil {
		return nil, err
	}
	return api.CreatePlacement201JSONResponse(toAPI(v)), nil
}

// UpdatePlacement handles PATCH /v1/placements/{placementId}.
func (h *HTTP) UpdatePlacement(ctx context.Context, req api.UpdatePlacementRequestObject) (api.UpdatePlacementResponseObject, error) {
	b := req.Body
	patch := Patch{
		RecruiterID:   c.Optional(b.RecruiterId, c.Same[string]),
		StartDate:     c.Required(dayPtr(b.StartDate)),
		GuaranteeDays: c.Required(b.GuaranteeDays),
		Status:        c.Required(c.Convert[string](b.Status)),
		LeftOn:        c.Optional(b.LeftOn, dayOf),
		StatusNote:    c.Optional(b.StatusNote, c.Same[string]),
		Replacement:   c.Required(c.Convert[string](b.Replacement)),
		InvoiceStatus: c.Required(c.Convert[string](b.InvoiceStatus)),
		InvoiceNumber: c.Optional(b.InvoiceNumber, c.Same[string]),
		InvoicedOn:    c.Optional(b.InvoicedOn, dayOf),
		PaidOn:        c.Optional(b.PaidOn, dayOf),
		Sheet: SheetPatch{
			JobTitle: given(b.JobTitle), CompanyName: given(b.CompanyName), JobType: c.Required(c.Convert[string](b.JobType)),
			PONumber: c.Optional(b.PoNumber, c.Same[string]), EndDate: c.Optional(b.EndDate, dayOf),
		},
	}
	var money c.Change[*Money]
	if b.Money.IsSpecified() {
		money.Set = true
		if m, err := b.Money.Get(); err == nil {
			money.Value = moneyIn(m)
		}
	}
	v, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.PlacementId, b.Version, patch, money)
	if err != nil {
		return nil, err
	}
	return api.UpdatePlacement200JSONResponse(toAPI(v)), nil
}

func dayOf(d openapi_types.Date) time.Time { return d.Time }

// given is a change to an optional text when it was sent ("" clears it).
func given(s *string) c.Change[*string] { return c.Change[*string]{Set: s != nil, Value: s} }

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

func moneyIn(m api.PlacementMoneyInput) *Money {
	out := &Money{FeePercent: c.ValueOrNil(m.FeePercent), FeeAmount: c.ValueOrNil(m.FeeAmount), FeeCurrency: c.ValueOrNil(m.FeeCurrency)}
	if s, err := m.Salary.Get(); err == nil {
		out.Salary = &Salary{Amount: s.Amount, Currency: s.Currency, Period: string(s.Period)}
	}
	return out
}

func toAPI(v View) api.Placement {
	d := v.Details
	out := api.Placement{
		Id: v.ID, Version: v.Version, ApplicationId: v.ApplicationID,
		CandidateId: v.App.CandidateID, CandidateName: v.App.CandidateName, JobId: v.App.JobID, JobTitle: v.App.JobTitle, CompanyId: v.App.CompanyID, CompanyName: v.App.CompanyName,
		JobType: api.PlacementJobType(d.JobType), PoNumber: c.OrNull(d.PONumber), EndDate: dateOf(d.EndDate),
		RecruiterId: c.OrNull(d.RecruiterID), RecruiterName: c.OrNull(v.RecruiterName),
		StartDate: openapi_types.Date{Time: d.StartDate}, GuaranteeDays: d.GuaranteeDays, GuaranteeEndsOn: openapi_types.Date{Time: d.GuaranteeEnds()},
		Status: api.PlacementStatus(d.Status), LeftOn: dateOf(d.LeftOn), StatusNote: c.OrNull(d.StatusNote),
		Replacement:   api.ReplacementStatus(d.Replacement),
		InvoiceStatus: api.InvoiceStatus(d.InvoiceStatus), InvoiceNumber: c.OrNull(d.InvoiceNumber),
		InvoicedOn: dateOf(d.InvoicedOn), PaidOn: dateOf(d.PaidOn),
		Invoicing:        invoicingToAPI(v.Invoicing),
		RestrictedFields: []api.PlacementRestrictedField{}, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	switch {
	case v.MoneyHidden:
		out.RestrictedFields = append(out.RestrictedFields, api.PlacementRestrictedFieldMoney)
	case v.Money == nil && v.Retainer == nil:
		out.Money = nullable.NewNullNullable[api.PlacementMoney]()
	default:
		money := Money{}
		if v.Money != nil {
			money = *v.Money
		}
		m := api.PlacementMoney{FeePercent: c.OrNull(money.FeePercent), FeeAmount: c.OrNull(money.FeeAmount), FeeCurrency: c.OrNull(money.FeeCurrency)}
		m.Retainer = placementRetainerToAPI(v.Retainer, money)
		m.Salary = nullable.NewNullNullable[api.SalaryAmount]()
		if s := money.Salary; s != nil {
			m.Salary = nullable.NewNullableWithValue(api.SalaryAmount{Amount: s.Amount, Currency: s.Currency, Period: api.PayPeriod(s.Period)})
		}
		out.Money = nullable.NewNullableWithValue(m)
	}
	return out
}
