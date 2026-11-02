package jobs

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// ListJobAds handles GET /v1/jobs/{jobId}/ads.
func (h *HTTP) ListJobAds(ctx context.Context, req api.ListJobAdsRequestObject) (api.ListJobAdsResponseObject, error) {
	ads, err := h.svc.ListAds(ctx, authz.MustFromContext(ctx), req.JobId)
	if err != nil {
		return nil, err
	}
	out := api.ListJobAds200JSONResponse{Items: make([]api.JobAd, len(ads))}
	for i, a := range ads {
		out.Items[i] = adToAPI(a)
	}
	return out, nil
}

// CreateJobAd handles POST /v1/jobs/{jobId}/ads.
func (h *HTTP) CreateJobAd(ctx context.Context, req api.CreateJobAdRequestObject) (api.CreateJobAdResponseObject, error) {
	b := req.Body
	ad, err := h.svc.CreateAd(ctx, authz.MustFromContext(ctx), req.JobId, AdDetails{
		Title: b.Title, Location: c.ValueOrNil(b.Location), WorkType: enumOrNil(b.WorkType),
		WorkArrangement: enumOrNil(b.WorkArrangement), SalaryText: c.ValueOrNil(b.SalaryText),
		Description: c.ValueOrNil(b.Description), ClosesOn: dateOrNil(c.ValueOrNil(b.ClosesOn)),
	})
	if err != nil {
		return nil, err
	}
	return api.CreateJobAd201JSONResponse(adToAPI(ad)), nil
}

// GetJobAd handles GET /v1/job-ads/{adId}.
func (h *HTTP) GetJobAd(ctx context.Context, req api.GetJobAdRequestObject) (api.GetJobAdResponseObject, error) {
	ad, err := h.svc.GetAd(ctx, authz.MustFromContext(ctx), req.AdId)
	if err != nil {
		return nil, err
	}
	return api.GetJobAd200JSONResponse(adToAPI(ad)), nil
}

// UpdateJobAd handles PATCH /v1/job-ads/{adId}.
func (h *HTTP) UpdateJobAd(ctx context.Context, req api.UpdateJobAdRequestObject) (api.UpdateJobAdResponseObject, error) {
	b := req.Body
	patch := AdPatch{
		Title:           c.Required(b.Title),
		Location:        c.Optional(b.Location, c.Same[string]),
		WorkType:        c.Optional(b.WorkType, func(v api.WorkType) string { return string(v) }),
		WorkArrangement: c.Optional(b.WorkArrangement, func(v api.WorkArrangement) string { return string(v) }),
		SalaryText:      c.Optional(b.SalaryText, c.Same[string]),
		Description:     c.Optional(b.Description, c.Same[string]),
		ClosesOn:        c.Optional(b.ClosesOn, func(d openapi_types.Date) time.Time { return d.Time }),
		Status:          c.Required(c.Convert[string](b.Status)),
	}
	ad, err := h.svc.UpdateAd(ctx, authz.MustFromContext(ctx), req.AdId, b.Version, patch)
	if err != nil {
		return nil, err
	}
	return api.UpdateJobAd200JSONResponse(adToAPI(ad)), nil
}

func adToAPI(a Ad) api.JobAd {
	d := a.Details
	out := api.JobAd{
		Id: a.ID, Version: a.Version, JobId: a.JobID, Title: d.Title, Status: api.JobAdStatus(d.Status),
		Location: c.OrNull(d.Location), SalaryText: c.OrNull(d.SalaryText), Description: c.OrNull(d.Description),
		PublishedAt: c.OrNull(a.PublishedAt), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		WorkType: c.OrNull(enumPtr[api.WorkType](d.WorkType)), WorkArrangement: c.OrNull(enumPtr[api.WorkArrangement](d.WorkArrangement)),
	}
	var closes *openapi_types.Date
	if d.ClosesOn != nil {
		closes = &openapi_types.Date{Time: *d.ClosesOn}
	}
	out.ClosesOn = c.OrNull(closes)
	return out
}

func enumOrNil[T ~string](n interface{ Get() (T, error) }) *string {
	v, err := n.Get()
	if err != nil {
		return nil
	}
	s := string(v)
	return &s
}

func enumPtr[T ~string](s *string) *T {
	if s == nil {
		return nil
	}
	v := T(*s)
	return &v
}

func dateOrNil(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}
