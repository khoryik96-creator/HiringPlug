package leads

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the leads operations of the contract.
type HTTP struct{ svc *Service }

// ListLeadSources handles GET /v1/lead-sources.
func (h *HTTP) ListLeadSources(ctx context.Context, _ api.ListLeadSourcesRequestObject) (api.ListLeadSourcesResponseObject, error) {
	rows, err := h.svc.Sources(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.ListLeadSources200JSONResponse{Items: make([]api.LeadSource, len(rows))}
	for i, r := range rows {
		out.Items[i] = sourceToAPI(r)
	}
	return out, nil
}

// CreateLeadSource handles POST /v1/lead-sources.
func (h *HTTP) CreateLeadSource(ctx context.Context, req api.CreateLeadSourceRequestObject) (api.CreateLeadSourceResponseObject, error) {
	b := req.Body
	name := ""
	if b.Name != nil {
		name = *b.Name
	}
	row, err := h.svc.CreateSource(ctx, authz.MustFromContext(ctx), string(b.Kind), b.Identifier, name, b.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.CreateLeadSource201JSONResponse(sourceToAPI(row)), nil
}

// UpdateLeadSource handles PATCH /v1/lead-sources/{leadSourceId}.
func (h *HTTP) UpdateLeadSource(ctx context.Context, req api.UpdateLeadSourceRequestObject) (api.UpdateLeadSourceResponseObject, error) {
	b := req.Body
	row, err := h.svc.UpdateSource(ctx, authz.MustFromContext(ctx), req.LeadSourceId, b.Version, SourcePatch{
		Name: c.Required(b.Name), CompanyID: c.Optional(b.CompanyId, c.Same[string]), Active: c.Required(b.Active),
	})
	if err != nil {
		return nil, err
	}
	return api.UpdateLeadSource200JSONResponse(sourceToAPI(row)), nil
}

// DeleteLeadSource handles DELETE /v1/lead-sources/{leadSourceId}.
func (h *HTTP) DeleteLeadSource(ctx context.Context, req api.DeleteLeadSourceRequestObject) (api.DeleteLeadSourceResponseObject, error) {
	if err := h.svc.DeleteSource(ctx, authz.MustFromContext(ctx), req.LeadSourceId); err != nil {
		return nil, err
	}
	return api.DeleteLeadSource204Response{}, nil
}

// CheckLeadSource handles POST /v1/lead-sources/{leadSourceId}/checks.
func (h *HTTP) CheckLeadSource(ctx context.Context, req api.CheckLeadSourceRequestObject) (api.CheckLeadSourceResponseObject, error) {
	row, err := h.svc.CheckNow(ctx, authz.MustFromContext(ctx), req.LeadSourceId)
	if err != nil {
		return nil, err
	}
	return api.CheckLeadSource202JSONResponse(sourceToAPI(row)), nil
}

// ListJobLeads handles GET /v1/job-leads.
func (h *HTTP) ListJobLeads(ctx context.Context, req api.ListJobLeadsRequestObject) (api.ListJobLeadsResponseObject, error) {
	q := req.Params
	rows, err := h.svc.Leads(ctx, authz.MustFromContext(ctx), LeadFilter{Status: c.Convert[string](q.Status), SourceID: q.SourceId, Open: q.Open, Q: trimmed(q.Q)})
	if err != nil {
		return nil, err
	}
	out := api.ListJobLeads200JSONResponse{Items: make([]api.JobLead, len(rows))}
	for i, r := range rows {
		out.Items[i] = leadToAPI(r)
	}
	return out, nil
}

// CreateJobLead handles POST /v1/job-leads.
func (h *HTTP) CreateJobLead(ctx context.Context, req api.CreateJobLeadRequestObject) (api.CreateJobLeadResponseObject, error) {
	b := req.Body
	row, err := h.svc.CreateLead(ctx, authz.MustFromContext(ctx), ManualLead{
		CompanyName: b.CompanyName, Title: b.Title, Location: b.Location, URL: b.Url, Notes: b.Notes, CompanyID: b.CompanyId,
	})
	if err != nil {
		return nil, err
	}
	return api.CreateJobLead201JSONResponse(leadToAPI(row)), nil
}

// UpdateJobLead handles PATCH /v1/job-leads/{jobLeadId}.
func (h *HTTP) UpdateJobLead(ctx context.Context, req api.UpdateJobLeadRequestObject) (api.UpdateJobLeadResponseObject, error) {
	b := req.Body
	row, err := h.svc.UpdateLead(ctx, authz.MustFromContext(ctx), req.JobLeadId, b.Version, LeadPatch{
		Status: c.Required(c.Convert[string](b.Status)), Notes: c.Optional(b.Notes, c.Same[string]), CompanyID: c.Optional(b.CompanyId, c.Same[string]),
	})
	if err != nil {
		return nil, err
	}
	return api.UpdateJobLead200JSONResponse(leadToAPI(row)), nil
}

func sourceToAPI(r store.LeadSource) api.LeadSource {
	return api.LeadSource{
		Id: r.ID, Version: int(r.Version), Kind: api.LeadSourceKind(r.Kind), Identifier: r.Identifier, Name: r.Name,
		CompanyId: c.OrNull(r.CompanyID), Active: r.Active, OpenLeads: int(r.OpenLeads),
		LastCheckedAt: c.OrNull(r.LastCheckedAt), LastError: c.OrNull(r.LastError), NextCheckAt: c.OrNull(r.NextCheckAt), CreatedAt: r.CreatedAt,
	}
}

func leadToAPI(r store.JobLead) api.JobLead {
	return api.JobLead{
		Id: r.ID, Version: int(r.Version), SourceId: c.OrNull(r.SourceID), SourceKind: c.OrNull(c.Convert[api.LeadSourceKind](r.SourceKind)),
		CompanyName: r.CompanyName, CompanyId: c.OrNull(r.CompanyID), Title: r.Title, Location: c.OrNull(r.Location), Url: c.OrNull(r.Url),
		PostedAt: c.OrNull(r.PostedAt), FirstSeenAt: r.FirstSeenAt, LastSeenAt: r.LastSeenAt, ClosedAt: c.OrNull(r.ClosedAt),
		Status: api.LeadStatus(r.Status), Notes: c.OrNull(r.Notes),
	}
}
