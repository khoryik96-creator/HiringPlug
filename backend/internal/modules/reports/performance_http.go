package reports

// HTTP handler for the performance report: contract types <-> service call.

import (
	"context"
	"math"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func perfRowToAPI(r PerfRow) api.PerformanceRow {
	fees := make([]api.MoneyTotal, len(r.Fees))
	for i, f := range r.Fees {
		fees[i] = api.MoneyTotal{Currency: f.Currency, Amount: f.Amount}
	}
	categories := r.Categories
	if categories == nil {
		categories = map[string]int{}
	}
	return api.PerformanceRow{From: openapi_types.Date{Time: r.From}, To: openapi_types.Date{Time: r.To}, CvsSent: r.CVsSent,
		Categories: categories, Placements: r.Placements, Fees: fees}
}

func percentOf(part, whole int) *int {
	if whole == 0 {
		return nil
	}
	p := int(math.Round(float64(part) * 100 / float64(whole)))
	return &p
}

func daysToAPI(avg *float64, jobs int) api.PerformanceDays {
	out := api.PerformanceDays{Jobs: jobs}
	if avg != nil {
		v := float32(*avg)
		out.Average.Set(v)
	} else {
		out.Average.SetNull()
	}
	return out
}

// GetPerformance handles GET /v1/reports/performance.
func (h *HTTP) GetPerformance(ctx context.Context, req api.GetPerformanceRequestObject) (api.GetPerformanceResponseObject, error) {
	grain := ""
	if req.Params.Grain != nil {
		grain = string(*req.Params.Grain)
	}
	var users []string
	if req.Params.User != nil {
		users = *req.Params.User
	}
	r, err := h.svc.Performance(ctx, authz.MustFromContext(ctx), grain, day(req.Params.From), day(req.Params.To), users)
	if err != nil {
		return nil, err
	}
	out := api.Performance{Grain: api.PerformanceGrain(r.Grain), From: openapi_types.Date{Time: r.From}, To: openapi_types.Date{Time: r.To},
		Users: r.Users, Team: r.Team, Rows: make([]api.PerformanceRow, len(r.Rows)), Total: perfRowToAPI(r.Total),
		Conversions: make([]api.PerformanceConversion, len(r.Conversions)), FeesHidden: r.FeesHidden,
		DaysToFirstCv: daysToAPI(r.DaysToFirstCV, r.JobsWithCV), DaysToFill: daysToAPI(r.DaysToFill, r.JobsFilled)}
	if out.Users == nil {
		out.Users = []string{}
	}
	for i, row := range r.Rows {
		out.Rows[i] = perfRowToAPI(row)
	}
	for i, c := range r.Conversions {
		conv := api.PerformanceConversion{From: c.From, To: c.To, Entered: c.Entered, Reached: c.Reached}
		if p := percentOf(c.Reached, c.Entered); p != nil {
			conv.Percent.Set(*p)
		} else {
			conv.Percent.SetNull()
		}
		out.Conversions[i] = conv
	}
	return api.GetPerformance200JSONResponse(out), nil
}
