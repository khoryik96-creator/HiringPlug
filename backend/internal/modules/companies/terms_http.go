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
	in := Terms{PaymentTermsDays: valueOf(b.PaymentTermsDays), GuaranteeDays: valueOf(b.GuaranteeDays), Notes: valueOf(b.Notes)}
	if v := valueOf(b.FeePercent); v != nil {
		s := string(*v)
		in.FeePercent = &s
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
		FeePercent: nullable.NewNullNullable[api.DecimalAmount](), PaymentTermsDays: nullOf(t.PaymentTermsDays),
		GuaranteeDays: nullOf(t.GuaranteeDays), SignedOn: nullable.NewNullNullable[openapi_types.Date](), Notes: nullOf(t.Notes),
		Version: t.Version, UpdatedAt: nullOf(t.UpdatedAt),
	}
	if t.FeePercent != nil {
		out.FeePercent = nullable.NewNullableWithValue(api.DecimalAmount(*t.FeePercent))
	}
	if t.SignedOn != nil {
		out.SignedOn = nullable.NewNullableWithValue(openapi_types.Date{Time: *t.SignedOn})
	}
	return out
}
