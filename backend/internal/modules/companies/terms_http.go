package companies

import (
	"context"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetCompanyTerms handles GET /v1/companies/{companyId}/terms.
func (h *HTTP) GetCompanyTerms(ctx context.Context, req api.GetCompanyTermsRequestObject) (api.GetCompanyTermsResponseObject, error) {
	t, err := h.svc.GetTerms(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.GetCompanyTerms200JSONResponse(termsToAPI(t)), nil
}

// PutCompanyTerms handles PUT /v1/companies/{companyId}/terms.
func (h *HTTP) PutCompanyTerms(ctx context.Context, req api.PutCompanyTermsRequestObject) (api.PutCompanyTermsResponseObject, error) {
	b := req.Body
	in := Terms{SalaryCurrency: b.SalaryCurrency, Notes: valueOf(b.Notes), Tiers: make([]Tier, len(b.Tiers))}
	for i, t := range b.Tiers {
		in.Tiers[i] = TierFromAPI(t)
	}
	if v := valueOf(b.SignedOn); v != nil {
		d := v.Time
		in.SignedOn = &d
	}
	t, err := h.svc.PutTerms(ctx, authz.MustFromContext(ctx), req.CompanyId, b.Version, in)
	if err != nil {
		return nil, err
	}
	return api.PutCompanyTerms200JSONResponse(termsToAPI(t)), nil
}

func termsToAPI(t Terms) api.CompanyTerms {
	out := api.CompanyTerms{
		Tiers: TiersToAPI(t.Tiers), SalaryCurrency: t.SalaryCurrency, SignedOn: nullable.NewNullNullable[openapi_types.Date](),
		Notes: nullOf(t.Notes), Version: t.Version, UpdatedAt: nullOf(t.UpdatedAt),
	}
	if out.SalaryCurrency == "" {
		out.SalaryCurrency = "MYR"
	}
	if t.SignedOn != nil {
		out.SignedOn = nullable.NewNullableWithValue(openapi_types.Date{Time: *t.SignedOn})
	}
	return out
}

// TierFromAPI reads a rate sent to the API.
func TierFromAPI(t api.CompanyTermsTier) Tier {
	return Tier{
		SalaryFrom: valueOf(t.SalaryFrom), SalaryBelow: valueOf(t.SalaryBelow), FeePercent: valueOf(t.FeePercent),
		PaymentTermsDays: valueOf(t.PaymentTermsDays), GuaranteeDays: valueOf(t.GuaranteeDays), Remarks: valueOf(t.Remarks),
	}
}

// TiersToAPI writes rates for the API (also the placements module's defaults).
func TiersToAPI(tiers []Tier) []api.CompanyTermsTier {
	out := make([]api.CompanyTermsTier, len(tiers))
	for i, t := range tiers {
		out[i] = api.CompanyTermsTier{
			SalaryFrom: nullOf(t.SalaryFrom), SalaryBelow: nullOf(t.SalaryBelow), FeePercent: nullOf(t.FeePercent),
			PaymentTermsDays: nullOf(t.PaymentTermsDays), GuaranteeDays: nullOf(t.GuaranteeDays), Remarks: nullOf(t.Remarks),
		}
	}
	return out
}
