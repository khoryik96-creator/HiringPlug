package companies

import (
	"bytes"
	"context"
	"mime"
	"time"

	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// GetCompanyBenefits handles GET /v1/companies/{companyId}/benefits.
func (h *HTTP) GetCompanyBenefits(ctx context.Context, req api.GetCompanyBenefitsRequestObject) (api.GetCompanyBenefitsResponseObject, error) {
	b, err := h.svc.GetBenefits(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.GetCompanyBenefits200JSONResponse(benefitsToAPI(b)), nil
}

// PutCompanyBenefits handles PUT /v1/companies/{companyId}/benefits.
func (h *HTTP) PutCompanyBenefits(ctx context.Context, req api.PutCompanyBenefitsRequestObject) (api.PutCompanyBenefitsResponseObject, error) {
	in := Benefits{Benefits: make([]Benefit, len(req.Body.Benefits)), PreOffer: make([]PreOfferStep, len(req.Body.PreOffer))}
	for i, b := range req.Body.Benefits {
		in.Benefits[i] = Benefit{Key: b.Key, Label: b.Label, Value: valueOf(b.Value), Remarks: valueOf(b.Remarks)}
	}
	for i, s := range req.Body.PreOffer {
		in.PreOffer[i] = PreOfferStep{Key: s.Key, Label: s.Label, Applies: valueOf(s.Applies), Remarks: valueOf(s.Remarks)}
	}
	b, err := h.svc.PutBenefits(ctx, authz.MustFromContext(ctx), req.CompanyId, req.Body.Version, in)
	if err != nil {
		return nil, err
	}
	return api.PutCompanyBenefits200JSONResponse(benefitsToAPI(b)), nil
}

// ExportCompanyBenefits handles GET /v1/companies/{companyId}/benefits/file.
func (h *HTTP) ExportCompanyBenefits(ctx context.Context, req api.ExportCompanyBenefitsRequestObject) (api.ExportCompanyBenefitsResponseObject, error) {
	e, err := h.svc.ExportBenefits(ctx, authz.MustFromContext(ctx), req.CompanyId, string(req.Params.Format))
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": e.FileName})
	return api.ExportCompanyBenefits200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(e.Data), ContentLength: int64(len(e.Data)),
		Headers: api.ExportCompanyBenefits200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func benefitsToAPI(b Benefits) api.CompanyBenefits {
	out := api.CompanyBenefits{
		Benefits: make([]api.CompanyBenefit, len(b.Benefits)), PreOffer: make([]api.CompanyPreOfferStep, len(b.PreOffer)),
		Version: b.Version, UpdatedAt: nullable.NewNullNullable[time.Time](),
	}
	if b.UpdatedAt != nil {
		out.UpdatedAt = nullable.NewNullableWithValue(*b.UpdatedAt)
	}
	for i, x := range b.Benefits {
		out.Benefits[i] = api.CompanyBenefit{Key: x.Key, Label: x.Label, Value: nullOf(x.Value), Remarks: nullOf(x.Remarks)}
	}
	for i, x := range b.PreOffer {
		out.PreOffer[i] = api.CompanyPreOfferStep{Key: x.Key, Label: x.Label, Applies: nullOf(x.Applies), Remarks: nullOf(x.Remarks)}
	}
	return out
}

// valueOf is a nullable's value, nil when null or not sent.
func valueOf[T any](n nullable.Nullable[T]) *T {
	if v, err := n.Get(); err == nil {
		return &v
	}
	return nil
}

func nullOf[T any](p *T) nullable.Nullable[T] {
	if p == nil {
		return nullable.NewNullNullable[T]()
	}
	return nullable.NewNullableWithValue(*p)
}
