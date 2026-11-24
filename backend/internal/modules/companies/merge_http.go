package companies

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// ListSimilarCompanies handles GET /v1/companies/{companyId}/similar.
func (h *HTTP) ListSimilarCompanies(ctx context.Context, req api.ListSimilarCompaniesRequestObject) (api.ListSimilarCompaniesResponseObject, error) {
	found, err := h.svc.Similar(ctx, authz.MustFromContext(ctx), req.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.ListSimilarCompanies200JSONResponse{Items: lookalikesToAPI(found)}, nil
}

// ListCompaniesNamedLike handles GET /v1/companies/named-like.
func (h *HTTP) ListCompaniesNamedLike(ctx context.Context, req api.ListCompaniesNamedLikeRequestObject) (api.ListCompaniesNamedLikeResponseObject, error) {
	found, err := h.svc.NamedLike(ctx, authz.MustFromContext(ctx), req.Params.Name)
	if err != nil {
		return nil, err
	}
	out := api.ListCompaniesNamedLike200JSONResponse{Items: make([]api.CompanyNamedLike, len(found))}
	for i, n := range found {
		out.Items[i] = api.CompanyNamedLike{Id: n.ID, Name: n.Name, City: c.OrNull(n.City), Website: c.OrNull(n.Website),
			LinkedinUrl: c.OrNull(n.LinkedinURL), Industry: c.OrNull(n.Industry), SameName: n.SameName}
	}
	return out, nil
}

// ListCompanyDuplicates handles GET /v1/companies/duplicates.
func (h *HTTP) ListCompanyDuplicates(ctx context.Context, _ api.ListCompanyDuplicatesRequestObject) (api.ListCompanyDuplicatesResponseObject, error) {
	groups, err := h.svc.Duplicates(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListCompanyDuplicates200JSONResponse{Items: make([]api.CompanyDuplicateGroup, len(groups))}
	for i, g := range groups {
		out.Items[i] = api.CompanyDuplicateGroup{Reasons: g.Reasons, Companies: lookalikesToAPI(g.Companies)}
	}
	return out, nil
}

// MergeCompany handles POST /v1/companies/{companyId}/merge.
func (h *HTTP) MergeCompany(ctx context.Context, req api.MergeCompanyRequestObject) (api.MergeCompanyResponseObject, error) {
	company, err := h.svc.Merge(ctx, authz.MustFromContext(ctx), req.CompanyId, req.Body.DuplicateId, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return api.MergeCompany200JSONResponse(toAPI(company)), nil
}

func lookalikesToAPI(found []Lookalike) []api.CompanyLookalike {
	out := make([]api.CompanyLookalike, len(found))
	for i, l := range found {
		out[i] = api.CompanyLookalike{Id: l.ID, Name: l.Name, City: c.OrNull(l.City), Website: c.OrNull(l.Website), Industry: c.OrNull(l.Industry), Reasons: l.Reasons}
	}
	return out
}
