package reports

// The activity and the revenue forecast in the reports file: activity in the
// report's period per user, and (for those who may see it) today's forecast
// per month, per recruiter and per application.

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/sheet"
)

// Extras are the activity in a period and, when p may see it, the forecast.
func (s *Service) Extras(ctx context.Context, p authz.Principal, first, last time.Time) (ActivityReport, *Forecast, error) {
	var activity ActivityReport
	var forecast *Forecast
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		if activity, err = s.activityIn(ctx, tx, p, first, last); err != nil {
			return err
		}
		if MayForecast(p) {
			f, err := s.forecastIn(ctx, tx, s.today())
			if err != nil {
				return err
			}
			forecast = &f
		}
		return nil
	})
	return activity, forecast, err
}

// ActivityTable is the activity sheet.
func ActivityTable(a ActivityReport) sheet.Table {
	t := sheet.Table{Name: "Activity", Columns: []sheet.Column{{Heading: "User"}, {Heading: "Calls", Kind: sheet.Number},
		{Heading: "CVs sent", Kind: sheet.Number}, {Heading: "Interviews arranged", Kind: sheet.Number}, {Heading: "Placements", Kind: sheet.Number}}}
	for _, r := range a.Rows {
		t.Rows = append(t.Rows, []string{r.Name, strconv.Itoa(r.Done.Calls), strconv.Itoa(r.Done.CVsSent), strconv.Itoa(r.Done.Interviews), strconv.Itoa(r.Done.Placements)})
	}
	return t
}

var forecastStageLabel = map[string]string{"submitted": "Submitted", "interview": "Interview", "offer": "Offer", "placed": "Placed"}

func monthText(m string) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return m
	}
	return t.Format("January 2006")
}

// ForecastTables are the forecast's sheets.
func ForecastTables(f Forecast) []sheet.Table {
	cols := func(first string) []sheet.Column {
		return []sheet.Column{{Heading: first}, {Heading: "Currency"}, {Heading: "Expected (weighted)", Kind: sheet.Number}, {Heading: "Full fees", Kind: sheet.Number}}
	}
	months := sheet.Table{Name: "Forecast by month", Columns: cols("Month")}
	for _, m := range f.Months {
		for _, t := range m.Totals {
			months.Rows = append(months.Rows, []string{monthText(m.Key), t.Currency, Money(t.Expected), Money(t.Full)})
		}
	}
	recruiters := sheet.Table{Name: "Forecast by recruiter", Columns: cols("Recruiter")}
	for _, r := range f.Recruiters {
		name := "No recruiter"
		if r.Key != "" {
			name = f.RecruiterNames[r.Key]
		}
		for _, t := range r.Totals {
			recruiters.Rows = append(recruiters.Rows, []string{name, t.Currency, Money(t.Expected), Money(t.Full)})
		}
	}
	lines := sheet.Table{Name: "Forecast detail", Columns: []sheet.Column{{Heading: "Candidate"}, {Heading: "Job"}, {Heading: "Client"},
		{Heading: "Stage"}, {Heading: "Recruiter"}, {Heading: "Month"}, {Heading: "Currency"}, {Heading: "Fee", Kind: sheet.Number},
		{Heading: "Weight %", Kind: sheet.Number}, {Heading: "Expected", Kind: sheet.Number}, {Heading: "Fee from"}}}
	for _, l := range f.Lines {
		recruiter, from := "", "Client's terms and the job's salary budget"
		if n := f.name(l.RecruiterID); n != nil {
			recruiter = *n
		}
		if l.FromPlacement {
			from = "The placement"
		}
		lines.Rows = append(lines.Rows, []string{l.CandidateName, l.JobTitle, l.CompanyName, forecastStageLabel[l.Stage], recruiter,
			monthText(l.Month), l.Currency, Money(l.Fee), strconv.Itoa(l.Percent), Money(l.Expected), from})
	}
	return []sheet.Table{months, recruiters, lines}
}
