package reports

// The revenue forecast: fees expected from the open pipeline and from
// placements not yet invoiced, weighted by stage. Fees are read as the user,
// so the database leaves out what they may not see; the forecast is refused
// without placements.fees:read. Not audited and never logged: it is a
// calculation over records, and its amounts are fees.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// At most this many lines and gaps are listed (all are added up).
const (
	maxLines = 2000
	maxGaps  = 500
)

// Forecast is the revenue forecast.
type Forecast struct {
	Months, Recruiters []Group
	RecruiterNames     map[string]string
	Lines              []Line
	Gaps               []Gap
	Weights            []StageWeight
}

// MayForecast reports whether p may see the forecast.
func MayForecast(p authz.Principal) bool {
	return p.Can(api.PermissionReportsForecastRead) && p.Can(api.PermissionPlacementsFeesRead)
}

// Forecast works out the revenue forecast as of today.
func (s *Service) Forecast(ctx context.Context, p authz.Principal) (Forecast, error) {
	if !MayForecast(p) {
		return Forecast{}, apperr.New(api.ErrorCodeFORBIDDEN, "The revenue forecast needs permission to see fees.")
	}
	var out Forecast
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.forecastIn(ctx, tx, s.today())
		return err
	})
	return out, err
}

func (s *Service) forecastIn(ctx context.Context, tx pgx.Tx, today time.Time) (Forecast, error) {
	src := s.deps.Forecast
	var out Forecast
	var err error
	if out.Weights, err = listWeights(ctx, tx); err != nil {
		return out, err
	}
	weight := map[string]StageWeight{}
	for _, w := range out.Weights {
		weight[w.Stage] = w
	}
	apps, err := src.AtStages(ctx, tx, ForecastStages)
	if err != nil {
		return out, err
	}
	uninvoiced, err := src.ToInvoice(ctx, tx)
	if err != nil {
		return out, err
	}
	placement := map[string]Uninvoiced{}
	for _, u := range uninvoiced {
		placement[u.ApplicationID] = u
	}
	// A placed application counts by its placement while that is not yet
	// invoiced; one invoiced (or fallen through) counts no more; one without
	// a placement counts as placed this month.
	var placedIDs, jobIDs []string
	for _, a := range apps {
		jobIDs = append(jobIDs, a.JobID)
		if _, ok := placement[a.ID]; a.Stage == "placed" && !ok {
			placedIDs = append(placedIDs, a.ID)
		}
	}
	withPlacement, err := src.WithPlacement(ctx, tx, placedIDs)
	if err != nil {
		return out, err
	}
	jobs, err := src.Jobs(ctx, tx, jobIDs)
	if err != nil {
		return out, err
	}
	terms := map[string]Terms{}
	var ids []string
	for _, a := range apps {
		u, placed := placement[a.ID]
		if a.Stage == "placed" && !placed && withPlacement[a.ID] {
			continue
		}
		job := jobs[a.JobID]
		w := weight[a.Stage]
		line := Line{ApplicationID: a.ID, CandidateName: a.CandidateName, JobTitle: a.JobTitle, CompanyName: a.CompanyName,
			Stage: a.Stage, RecruiterID: job.OwnerID, Percent: w.Percent}
		var start *time.Time
		if placed {
			start = &u.StartDate
			if u.RecruiterID != nil {
				line.RecruiterID = u.RecruiterID
			}
		}
		line.Month = ForecastMonth(today, start, w.MonthsAhead)
		if placed && u.FeeAmount != nil && u.FeeCurrency != nil {
			line.Fee, line.Currency, line.FromPlacement = rat(u.FeeAmount), *u.FeeCurrency, true
		}
		if line.Fee == nil {
			t, ok := terms[job.CompanyID]
			if !ok && job.CompanyID != "" {
				if t, err = src.Terms(ctx, tx, job.CompanyID); err != nil {
					return out, err
				}
				terms[job.CompanyID] = t
			}
			fee, currency, gap := ExpectedFee(job.Budget, t)
			if gap != "" {
				if len(out.Gaps) < maxGaps {
					out.Gaps = append(out.Gaps, Gap{ApplicationID: a.ID, CandidateName: a.CandidateName, JobTitle: a.JobTitle, Stage: a.Stage, Reason: gap})
				}
				continue
			}
			line.Fee, line.Currency = fee, currency
		}
		line.Expected = Weighted(line.Fee, line.Percent)
		out.Lines = append(out.Lines, line)
		if line.RecruiterID != nil {
			ids = append(ids, *line.RecruiterID)
		}
	}
	out.Months, out.Recruiters = ByMonth(out.Lines), ByRecruiter(out.Lines)
	SortLines(out.Lines)
	if len(out.Lines) > maxLines {
		out.Lines = out.Lines[:maxLines]
	}
	out.RecruiterNames, err = s.deps.Users.UserNames(ctx, tx, ids)
	return out, err
}
