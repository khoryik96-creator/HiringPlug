package companies

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

const defaultPageSize = 25

// HTTP implements the companies operations of the contract.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

// ListCompanies handles GET /v1/companies.
func (h *HTTP) ListCompanies(ctx context.Context, req api.ListCompaniesRequestObject) (api.ListCompaniesResponseObject, error) {
	limit := defaultPageSize
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	page, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.Params.Q, c.Convert[string](req.Params.Type), req.Params.Cursor, limit)
	if err != nil {
		return nil, err
	}
	out := api.ListCompanies200JSONResponse{Items: make([]api.CompanySummary, len(page.Items)), NextCursor: c.OrNull(page.NextCursor)}
	for i, s := range page.Items {
		out.Items[i] = api.CompanySummary{
			Id: s.ID, Name: s.Name, Type: api.CompanyType(s.Type), Industry: c.OrNull(s.Industry),
			City: c.OrNull(s.City), Website: c.OrNull(s.Website), UpdatedAt: s.UpdatedAt,
		}
	}
	return out, nil
}

// CreateCompany handles POST /v1/companies.
func (h *HTTP) CreateCompany(ctx context.Context, req api.CreateCompanyRequestObject) (api.CreateCompanyResponseObject, error) {
	b := req.Body
	in := Details{
		Name: b.Name, Website: c.ValueOrNil(b.Website), Industry: c.ValueOrNil(b.Industry),
		Size: c.Convert[string](c.ValueOrNil(b.Size)), Phone: c.ValueOrNil(b.Phone), Address: c.ValueOrNil(b.Address),
		City: c.ValueOrNil(b.City), Country: c.ValueOrNil(b.Country),
		RegistrationNumber: c.ValueOrNil(b.RegistrationNumber), About: c.ValueOrNil(b.About),
	}
	if b.Type != nil {
		in.Type = string(*b.Type)
	}
	company, err := h.svc.Create(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	return api.CreateCompany201JSONResponse(toAPI(company)), nil
}

// GetCompany handles GET /v1/companies/{companyId}.
func (h *HTTP) GetCompany(ctx context.Context, req api.GetCompanyRequestObject) (api.GetCompanyResponseObject, error) {
	company, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.GetCompany200JSONResponse(toAPI(company)), nil
}

// UpdateCompany handles PATCH /v1/companies/{companyId}.
func (h *HTTP) UpdateCompany(ctx context.Context, req api.UpdateCompanyRequestObject) (api.UpdateCompanyResponseObject, error) {
	b := req.Body
	patch := Patch{
		Name:               c.Required(b.Name),
		Type:               c.Required(c.Convert[string](b.Type)),
		Website:            c.Optional(b.Website, c.Same[string]),
		Industry:           c.Optional(b.Industry, c.Same[string]),
		Size:               c.Optional(b.Size, func(s api.CompanySize) string { return string(s) }),
		Phone:              c.Optional(b.Phone, c.Same[string]),
		Address:            c.Optional(b.Address, c.Same[string]),
		City:               c.Optional(b.City, c.Same[string]),
		Country:            c.Optional(b.Country, c.Same[string]),
		RegistrationNumber: c.Optional(b.RegistrationNumber, c.Same[string]),
		About:              c.Optional(b.About, c.Same[string]),
	}
	company, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.CompanyId, b.Version, patch)
	if err != nil {
		return nil, err
	}
	return api.UpdateCompany200JSONResponse(toAPI(company)), nil
}

func toAPI(co Company) api.Company {
	d := co.Details
	return api.Company{
		Id: co.ID, Version: co.Version, Name: d.Name, Type: api.CompanyType(d.Type),
		Website: c.OrNull(d.Website), Industry: c.OrNull(d.Industry), Size: c.OrNull(c.Convert[api.CompanySize](d.Size)),
		Phone: c.OrNull(d.Phone), Address: c.OrNull(d.Address), City: c.OrNull(d.City), Country: c.OrNull(d.Country),
		RegistrationNumber: c.OrNull(d.RegistrationNumber), About: c.OrNull(d.About),
		CreatedAt: co.CreatedAt, UpdatedAt: co.UpdatedAt,
	}
}
