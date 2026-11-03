package reports

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// HTTP implements the reports operations of the contract.
type HTTP struct{ svc *Service }

func day(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}

// GetReport handles GET /v1/reports/summary.
func (h *HTTP) GetReport(ctx context.Context, req api.GetReportRequestObject) (api.GetReportResponseObject, error) {
	r, err := h.svc.Report(ctx, authz.MustFromContext(ctx), day(req.Params.From), day(req.Params.To))
	if err != nil {
		return nil, err
	}
	out := api.Report{
		From: openapi_types.Date{Time: r.From}, To: openapi_types.Date{Time: r.To}, FeesHidden: r.FeesHidden,
		Funnel: make([]api.FunnelStage, len(r.Funnel)), Sources: make([]api.SourceRow, len(r.Sources)),
		Recruiters: make([]api.RecruiterRow, len(r.Recruiters)),
		TimeToFill: api.TimeToFill{
			MedianDaysToFill: c.OrNull(r.TimeToFill.MedianDaysToFill), MedianDaysToPlace: c.OrNull(r.TimeToFill.MedianDaysToPlace),
			Jobs: make([]api.JobFill, len(r.TimeToFill.Jobs)),
		},
	}
	for i, f := range r.Funnel {
		out.Funnel[i] = api.FunnelStage{Stage: api.Stage(f.Stage), Reached: f.Reached, PercentOfPrevious: c.OrNull(f.PercentOfPrevious)}
	}
	for i, s := range r.Sources {
		out.Sources[i] = api.SourceRow{Source: s.Source, Applications: s.Applications, Interviewed: s.Interviewed, Placed: s.Placed}
	}
	for i, rr := range r.Recruiters {
		row := api.RecruiterRow{RecruiterId: c.OrNull(rr.RecruiterID), Placements: rr.Placements, FellThrough: rr.FellThrough, Fees: []api.MoneyTotal{}}
		var name *string
		if rr.RecruiterID != nil {
			if n, ok := r.RecruiterNames[*rr.RecruiterID]; ok {
				name = &n
			}
		}
		row.RecruiterName = c.OrNull(name)
		for _, f := range rr.Fees {
			row.Fees = append(row.Fees, api.MoneyTotal{Currency: f.Currency, Amount: f.Amount})
		}
		out.Recruiters[i] = row
	}
	for i, j := range r.TimeToFill.Jobs {
		out.TimeToFill.Jobs[i] = api.JobFill{JobId: j.JobID, JobTitle: r.JobTitles[j.JobID], DaysToFill: j.DaysToFill, Placements: j.Placements}
	}
	return api.GetReport200JSONResponse(out), nil
}
