package industries

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP converts contract requests to tenant-scoped catalogue use cases.
type HTTP struct{ svc *Service }

func (h *HTTP) ListIndustries(ctx context.Context, _ api.ListIndustriesRequestObject) (api.ListIndustriesResponseObject, error) {
	c, e := h.svc.List(ctx, authz.MustFromContext(ctx))
	return api.ListIndustries200JSONResponse(c), e
}
func (h *HTTP) CreateIndustry(ctx context.Context, r api.CreateIndustryRequestObject) (api.CreateIndustryResponseObject, error) {
	c, e := h.svc.Edit(ctx, authz.MustFromContext(ctx), "", *r.Body)
	return api.CreateIndustry201JSONResponse(c), e
}
func (h *HTTP) UpdateIndustry(ctx context.Context, r api.UpdateIndustryRequestObject) (api.UpdateIndustryResponseObject, error) {
	c, e := h.svc.Edit(ctx, authz.MustFromContext(ctx), r.IndustryId, *r.Body)
	return api.UpdateIndustry200JSONResponse(c), e
}
func (h *HTTP) RemoveIndustry(ctx context.Context, r api.RemoveIndustryRequestObject) (api.RemoveIndustryResponseObject, error) {
	c, e := h.svc.Remove(ctx, authz.MustFromContext(ctx), r.IndustryId, *r.Body)
	return api.RemoveIndustry200JSONResponse(c), e
}
func (h *HTTP) SuggestCandidateIndustry(ctx context.Context, r api.SuggestCandidateIndustryRequestObject) (api.SuggestCandidateIndustryResponseObject, error) {
	c, e := h.svc.Suggest(ctx, authz.MustFromContext(ctx), r.CandidateId, r.Params.EmployerIndex)
	return api.SuggestCandidateIndustry200JSONResponse(c), e
}
