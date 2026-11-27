package reports

// HTTP handlers for activity against targets, the targets and weights, and
// the revenue forecast: contract types <-> service calls.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

func goalToAPI(g Goal) api.ActivityGoal {
	out := api.ActivityGoal{Calls: c.OrNull(g.Calls), CvsSent: c.OrNull(g.CVsSent), Interviews: c.OrNull(g.Interviews), Placements: c.OrNull(g.Placements)}
	if len(g.Categories) > 0 {
		out.Categories = &g.Categories
	}
	return out
}

func goalFromAPI(g api.ActivityGoal) Goal {
	out := Goal{Calls: c.ValueOrNil(g.Calls), CVsSent: c.ValueOrNil(g.CvsSent), Interviews: c.ValueOrNil(g.Interviews), Placements: c.ValueOrNil(g.Placements)}
	if g.Categories != nil && len(*g.Categories) > 0 {
		out.Categories = *g.Categories
	}
	return out
}

func countsToAPI(n Counts) api.ActivityCounts {
	categories := n.Categories
	if categories == nil {
		categories = map[string]int{}
	}
	return api.ActivityCounts{Calls: n.Calls, CvsSent: n.CVsSent, Interviews: n.Interviews, Placements: n.Placements, Categories: categories}
}

// ActivityToAPI is an activity report as the contract has it.
func ActivityToAPI(r ActivityReport) api.ActivityReport {
	out := api.ActivityReport{Period: api.ActivityPeriod(r.Period), From: openapi_types.Date{Time: r.From}, To: openapi_types.Date{Time: r.To},
		Team: r.Team, Rows: make([]api.ActivityRow, len(r.Rows))}
	for i, row := range r.Rows {
		a := api.ActivityRow{UserId: row.UserID, UserName: row.Name, Done: countsToAPI(row.Done)}
		if row.Target != nil {
			a.Target.Set(goalToAPI(*row.Target))
		} else {
			a.Target.SetNull()
		}
		out.Rows[i] = a
	}
	return out
}

// GetActivity handles GET /v1/reports/activity.
func (h *HTTP) GetActivity(ctx context.Context, req api.GetActivityRequestObject) (api.GetActivityResponseObject, error) {
	period := Week
	if req.Params.Period != nil {
		period = string(*req.Params.Period)
	}
	r, err := h.svc.Activity(ctx, authz.MustFromContext(ctx), period, day(req.Params.Day))
	if err != nil {
		return nil, err
	}
	return api.GetActivity200JSONResponse(ActivityToAPI(r)), nil
}

func settingsToAPI(s Settings) api.ReportSettings {
	out := api.ReportSettings{Targets: make([]api.ActivityTarget, len(s.Targets)), StageWeights: make([]api.StageWeight, len(s.Weights))}
	for i, t := range s.Targets {
		out.Targets[i] = api.ActivityTarget{UserId: c.OrNull(t.UserID), Period: api.ActivityPeriod(t.Period), Goal: goalToAPI(t.Goal)}
	}
	for i, w := range s.Weights {
		out.StageWeights[i] = api.StageWeight{Stage: api.ForecastStage(w.Stage), Percent: w.Percent, MonthsAhead: w.MonthsAhead}
	}
	return out
}

// GetReportSettings handles GET /v1/reports/settings.
func (h *HTTP) GetReportSettings(ctx context.Context, _ api.GetReportSettingsRequestObject) (api.GetReportSettingsResponseObject, error) {
	s, err := h.svc.Settings(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return api.GetReportSettings200JSONResponse(settingsToAPI(s)), nil
}

// PutReportSettings handles PUT /v1/reports/settings.
func (h *HTTP) PutReportSettings(ctx context.Context, req api.PutReportSettingsRequestObject) (api.PutReportSettingsResponseObject, error) {
	in := Settings{Targets: make([]Target, len(req.Body.Targets)), Weights: make([]StageWeight, len(req.Body.StageWeights))}
	for i, t := range req.Body.Targets {
		in.Targets[i] = Target{UserID: c.ValueOrNil(t.UserId), Period: string(t.Period), Goal: goalFromAPI(t.Goal)}
	}
	for i, w := range req.Body.StageWeights {
		in.Weights[i] = StageWeight{Stage: string(w.Stage), Percent: w.Percent, MonthsAhead: w.MonthsAhead}
	}
	s, err := h.svc.SaveSettings(ctx, authz.MustFromContext(ctx), in)
	if err != nil {
		return nil, err
	}
	return api.PutReportSettings200JSONResponse(settingsToAPI(s)), nil
}

func totalsToAPI(ts []Total) []api.ForecastTotal {
	out := make([]api.ForecastTotal, len(ts))
	for i, t := range ts {
		out[i] = api.ForecastTotal{Currency: t.Currency, Expected: Money(t.Expected), Full: Money(t.Full)}
	}
	return out
}

func (f Forecast) name(id *string) *string {
	if id == nil {
		return nil
	}
	if n, ok := f.RecruiterNames[*id]; ok {
		return &n
	}
	return nil
}

// GetForecast handles GET /v1/reports/forecast.
func (h *HTTP) GetForecast(ctx context.Context, _ api.GetForecastRequestObject) (api.GetForecastResponseObject, error) {
	f, err := h.svc.Forecast(ctx, authz.MustFromContext(ctx))
	if err != nil {
		return nil, err
	}
	out := api.Forecast{Months: make([]api.ForecastMonth, len(f.Months)), Recruiters: make([]api.ForecastRecruiter, len(f.Recruiters)),
		Lines: make([]api.ForecastLine, len(f.Lines)), Gaps: make([]api.ForecastGap, len(f.Gaps)), StageWeights: settingsToAPI(Settings{Weights: f.Weights}).StageWeights}
	for i, m := range f.Months {
		out.Months[i] = api.ForecastMonth{Month: m.Key, Totals: totalsToAPI(m.Totals)}
	}
	for i, r := range f.Recruiters {
		var id *string
		if r.Key != "" {
			id = &r.Key
		}
		out.Recruiters[i] = api.ForecastRecruiter{RecruiterId: c.OrNull(id), RecruiterName: c.OrNull(f.name(id)), Totals: totalsToAPI(r.Totals)}
	}
	for i, l := range f.Lines {
		from := api.ForecastLineFeeFromTerms
		if l.FromPlacement {
			from = api.ForecastLineFeeFromPlacement
		}
		out.Lines[i] = api.ForecastLine{ApplicationId: l.ApplicationID, CandidateName: l.CandidateName, JobTitle: l.JobTitle, CompanyName: l.CompanyName,
			Stage: api.ForecastStage(l.Stage), RecruiterId: c.OrNull(l.RecruiterID), RecruiterName: c.OrNull(f.name(l.RecruiterID)), Month: l.Month,
			Currency: l.Currency, Fee: Money(l.Fee), Percent: l.Percent, Expected: Money(l.Expected), FeeFrom: from}
	}
	for i, g := range f.Gaps {
		out.Gaps[i] = api.ForecastGap{ApplicationId: g.ApplicationID, CandidateName: g.CandidateName, JobTitle: g.JobTitle,
			Stage: api.ForecastStage(g.Stage), Reason: api.ForecastGapReason(g.Reason)}
	}
	return api.GetForecast200JSONResponse(out), nil
}
