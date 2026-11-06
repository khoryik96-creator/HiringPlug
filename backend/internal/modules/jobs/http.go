package jobs

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

const defaultPageSize = 25

// HTTP implements the jobs operations of the contract.
type HTTP struct {
	svc            *Service
	maxUploadBytes int64
}

// ListJobs handles GET /v1/jobs.
func (h *HTTP) ListJobs(ctx context.Context, req api.ListJobsRequestObject) (api.ListJobsResponseObject, error) {
	limit := defaultPageSize
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	page, err := h.svc.List(ctx, authz.MustFromContext(ctx), req.Params.Q, c.Convert[string](req.Params.Status), req.Params.CompanyId, req.Params.Cursor, limit)
	if err != nil {
		return nil, err
	}
	out := api.ListJobs200JSONResponse{Items: make([]api.JobOrderSummary, len(page.Items)), NextCursor: c.OrNull(page.NextCursor)}
	for i, s := range page.Items {
		out.Items[i] = api.JobOrderSummary{
			Id: s.ID, Title: s.Title, CompanyId: s.CompanyID, CompanyName: s.CompanyName, Status: api.JobStatus(s.Status),
			Location: c.OrNull(s.Location), Openings: s.Openings, UpdatedAt: s.UpdatedAt,
		}
	}
	return out, nil
}

// CreateJob handles POST /v1/jobs.
func (h *HTTP) CreateJob(ctx context.Context, req api.CreateJobRequestObject) (api.CreateJobResponseObject, error) {
	b := req.Body
	in := Details{
		CompanyID: b.CompanyId, ContactID: c.ValueOrNil(b.ContactId), Title: b.Title, Location: c.ValueOrNil(b.Location),
		WorkType:        c.Convert[string](c.ValueOrNil(b.WorkType)),
		WorkArrangement: c.Convert[string](c.ValueOrNil(b.WorkArrangement)),
		Description:     c.ValueOrNil(b.Description),
		Country:         c.ValueOrNil(b.Country),
	}
	if b.IntakeDate != nil {
		in.IntakeDate = b.IntakeDate.Time
	}
	if b.Status != nil {
		in.Status = string(*b.Status)
	}
	if b.Openings != nil {
		in.Openings = *b.Openings
	}
	view, err := h.svc.Create(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	return api.CreateJob201JSONResponse(toAPI(view)), nil
}

// GetJob handles GET /v1/jobs/{jobId}.
func (h *HTTP) GetJob(ctx context.Context, req api.GetJobRequestObject) (api.GetJobResponseObject, error) {
	view, err := h.svc.Get(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	return api.GetJob200JSONResponse(toAPI(view)), nil
}

// UpdateJob handles PATCH /v1/jobs/{jobId}.
func (h *HTTP) UpdateJob(ctx context.Context, req api.UpdateJobRequestObject) (api.UpdateJobResponseObject, error) {
	b := req.Body
	patch := Patch{
		CompanyID:       c.Required(b.CompanyId),
		ContactID:       c.Optional(b.ContactId, c.Same[string]),
		Title:           c.Required(b.Title),
		Status:          c.Required(c.Convert[string](b.Status)),
		Location:        c.Optional(b.Location, c.Same[string]),
		WorkType:        c.Optional(b.WorkType, func(v api.WorkType) string { return string(v) }),
		WorkArrangement: c.Optional(b.WorkArrangement, func(v api.WorkArrangement) string { return string(v) }),
		Openings:        c.Required(b.Openings),
		Description:     c.Optional(b.Description, c.Same[string]),
		Country:         c.Optional(b.Country, c.Same[string]),
		IntakeDate:      c.Required(dateOrNil(b.IntakeDate)),
	}
	view, err := h.svc.Update(ctx, authz.MustFromContext(ctx), req.JobId, b.Version, patch)
	if err != nil {
		return nil, err
	}
	return api.UpdateJob200JSONResponse(toAPI(view)), nil
}

// PutJobSalary handles PUT /v1/jobs/{jobId}/salary.
func (h *HTTP) PutJobSalary(ctx context.Context, req api.PutJobSalaryRequestObject) (api.PutJobSalaryResponseObject, error) {
	b := req.Body
	var r *SalaryRange
	if v, err := b.Range.Get(); err == nil {
		r = &SalaryRange{Min: v.Min, Max: c.ValueOrNil(v.Max), Currency: v.Currency, Period: string(v.Period)}
	}
	salary, err := h.svc.SetSalary(ctx, authz.MustFromContext(ctx), req.JobId, b.ExpectedVersion, r)
	if err != nil {
		return nil, err
	}
	if salary == nil { // cleared, or nothing to clear
		return api.PutJobSalary200JSONResponse{Version: 0, Range: c.OrNull[api.JobSalaryRange](nil)}, nil
	}
	return api.PutJobSalary200JSONResponse(salaryToAPI(*salary)), nil
}

func toAPI(v View) api.JobOrder {
	d := v.Details
	out := api.JobOrder{
		Id: v.ID, Version: v.Version, CompanyId: d.CompanyID, CompanyName: v.CompanyName,
		ContactId: c.OrNull(d.ContactID), ContactName: c.OrNull(v.ContactName), Title: d.Title,
		Status: api.JobStatus(d.Status), Location: c.OrNull(d.Location),
		WorkType:        c.OrNull(c.Convert[api.WorkType](d.WorkType)),
		WorkArrangement: c.OrNull(c.Convert[api.WorkArrangement](d.WorkArrangement)),
		Openings:        d.Openings, Description: c.OrNull(d.Description),
		Country: c.OrNull(d.Country), IntakeDate: openapi_types.Date{Time: d.IntakeDate},
		RestrictedFields: []api.JobRestrictedField{}, OwnerId: c.OrNull(v.OwnerID), OwnerName: c.OrNull(v.OwnerName),
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	if v.SalaryRestricted {
		out.RestrictedFields = append(out.RestrictedFields, api.JobRestrictedFieldSalary)
		return out // salary left out entirely
	}
	var salary *api.JobSalary
	if v.Salary != nil {
		s := salaryToAPI(*v.Salary)
		salary = &s
	}
	out.Salary = c.OrNull(salary)
	return out
}

func salaryToAPI(s Salary) api.JobSalary {
	var rng *api.JobSalaryRange
	if r := s.Range; r != nil {
		rng = &api.JobSalaryRange{Min: r.Min, Max: c.OrNull(r.Max), Currency: r.Currency, Period: api.PayPeriod(r.Period)}
	}
	return api.JobSalary{Version: s.Version, Range: c.OrNull(rng), UpdatedAt: s.UpdatedAt}
}

// SetJobOwner handles PUT /v1/jobs/{jobId}/owner.
func (h *HTTP) SetJobOwner(ctx context.Context, req api.SetJobOwnerRequestObject) (api.SetJobOwnerResponseObject, error) {
	var owner *string
	if v, err := req.Body.OwnerId.Get(); err == nil {
		owner = &v
	}
	if err := h.svc.SetOwner(ctx, authz.MustFromContext(ctx), req.JobId, owner); err != nil {
		return nil, err
	}
	return api.SetJobOwner204Response{}, nil
}
