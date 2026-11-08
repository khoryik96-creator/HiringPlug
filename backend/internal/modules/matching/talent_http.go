package matching

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// StartTalentSearch handles POST /v1/talent-searches.
func (h *HTTP) StartTalentSearch(ctx context.Context, req api.StartTalentSearchRequestObject) (api.StartTalentSearchResponseObject, error) {
	p := authz.MustFromContext(ctx)
	row, err := h.svc.StartTalentSearch(ctx, p, *req.Body)
	if err != nil {
		return nil, err
	}
	out, err := h.svc.TalentSearch(ctx, p, row.ID)
	if err != nil {
		return nil, err
	}
	return api.StartTalentSearch202JSONResponse(out), nil
}

// GetTalentSearch handles GET /v1/talent-searches/{searchId}.
func (h *HTTP) GetTalentSearch(ctx context.Context, req api.GetTalentSearchRequestObject) (api.GetTalentSearchResponseObject, error) {
	out, err := h.svc.TalentSearch(ctx, authz.MustFromContext(ctx), req.SearchId)
	if err != nil {
		return nil, err
	}
	return api.GetTalentSearch200JSONResponse(out), nil
}

// ListTalentSearches handles GET /v1/talent-searches.
func (h *HTTP) ListTalentSearches(ctx context.Context, _ api.ListTalentSearchesRequestObject) (api.ListTalentSearchesResponseObject, error) {
	items, err := h.svc.TalentSearches(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.ListTalentSearches200JSONResponse{Items: items}, nil
}
