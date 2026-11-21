package leads

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func asWorkspaceLead(r store.JobLead, favorite bool) api.JobLead {
	value := leadToAPI(r)
	value.Favorite = &favorite
	return value
}

// ListJobLeads handles GET /v1/job-leads.
func (h *HTTP) ListJobLeads(ctx context.Context, req api.ListJobLeadsRequestObject) (api.ListJobLeadsResponseObject, error) {
	q := req.Params
	limit := 500
	if q.Limit != nil {
		limit = min(500, max(1, *q.Limit))
	}
	rows, err := h.svc.workspace(ctx, authz.MustFromContext(ctx), store.ListLeadsParams{
		Status: c.Convert[string](q.Status), SourceID: q.SourceId, Open: q.Open, Q: trimmed(q.Q),
		Cursor: q.Cursor, Favorites: q.Favorites != nil && *q.Favorites, RowLimit: int32(limit + 1), //nolint:gosec // validated page size
	})
	if err != nil {
		return nil, err
	}
	rows, next := pageLeads(rows, limit, func(r store.ListLeadsRow) string { return r.JobLead.ID })
	out := api.ListJobLeads200JSONResponse{Items: make([]api.JobLead, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = asWorkspaceLead(r.JobLead, r.Favorite)
	}
	return out, nil
}

// GetJobLead handles GET /v1/job-leads/{jobLeadId}.
func (h *HTTP) GetJobLead(ctx context.Context, req api.GetJobLeadRequestObject) (api.GetJobLeadResponseObject, error) {
	view, err := h.svc.leadDetail(ctx, authz.MustFromContext(ctx), req.JobLeadId)
	if err != nil {
		return nil, err
	}
	lead := asWorkspaceLead(view.lead, view.favorite)
	if view.evidence != nil {
		lead.Description = &view.evidence.Description
		lead.DescriptionObservedAt = &view.evidence.ObservedAt
	}
	return api.GetJobLead200JSONResponse(lead), nil
}

// SetJobLeadFavorite handles PUT /v1/job-leads/{jobLeadId}/favorite.
func (h *HTTP) SetJobLeadFavorite(ctx context.Context, req api.SetJobLeadFavoriteRequestObject) (api.SetJobLeadFavoriteResponseObject, error) {
	if err := h.svc.setFavorite(ctx, authz.MustFromContext(ctx), req.JobLeadId, req.Body.Favorite); err != nil {
		return nil, err
	}
	return api.SetJobLeadFavorite204Response{}, nil
}

// DiscoverJobLeads handles POST /v1/job-leads/discover.
func (h *HTTP) DiscoverJobLeads(ctx context.Context, req api.DiscoverJobLeadsRequestObject) (api.DiscoverJobLeadsResponseObject, error) {
	body := req.Body
	in := discoveryInput{candidateID: body.CandidateId, documentID: body.DocumentId, cursor: body.Cursor}
	for _, country := range body.Countries {
		in.countries = append(in.countries, string(country))
	}
	if body.Sources != nil {
		for _, source := range *body.Sources {
			in.sources = append(in.sources, string(source))
		}
	}
	if body.TargetRole != nil {
		in.target = *body.TargetRole
	}
	role, rows, next, err := h.svc.discover(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	out := api.DiscoverJobLeads200JSONResponse{Items: make([]api.JobLead, len(rows)), Role: role, NextCursor: next}
	for i, row := range rows {
		out.Items[i] = asWorkspaceLead(row.lead, row.favorite)
	}
	return out, nil
}
