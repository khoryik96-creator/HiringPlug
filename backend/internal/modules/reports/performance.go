package reports

// The performance report: what the chosen people (or the whole team) did and
// brought in over a range, by week, month, quarter or year, with conversions
// between stage categories and how long jobs took to their first CV and to
// their first placement.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// MaxPerformanceDays is the longest range a performance report covers.
const MaxPerformanceDays = 5 * 366

// PerformanceReport is the performance report.
type PerformanceReport struct {
	Grain         string
	From, To      time.Time
	Users         []string // empty: the whole team
	Team          bool     // may see others' numbers
	Rows          []PerfRow
	Total         PerfRow
	Conversions   []Conversion
	DaysToFirstCV *float64
	JobsWithCV    int
	DaysToFill    *float64
	JobsFilled    int
	FeesHidden    bool
}

// Performance works out the performance report for the days from..to (this
// year so far by default) split by grain, for users (the whole team when
// none; only yourself without reports.team:read).
func (s *Service) Performance(ctx context.Context, p authz.Principal, grain string, from, to *time.Time, users []string) (PerformanceReport, error) {
	last := s.today()
	if to != nil {
		last = *to
	}
	first := time.Date(last.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	if from != nil {
		first = *from
	}
	if grain == "" {
		grain = Month
	}
	buckets, errs := Buckets(grain, first, last)
	if errs == nil && last.Sub(first) > MaxPerformanceDays*24*time.Hour {
		errs.Add("from", "a report covers at most five years")
	}
	if err := errs.Err(); err != nil {
		return PerformanceReport{}, err
	}
	out := PerformanceReport{Grain: grain, From: first, To: last, Team: p.Can(api.PermissionReportsTeamRead),
		FeesHidden: !p.Can(api.PermissionPlacementsFeesRead), Users: users}
	if !out.Team {
		out.Users = []string{p.UserID}
	}
	chosen := map[string]bool{}
	for _, u := range out.Users {
		chosen[u] = true
	}
	counts := func(id string) bool { return len(chosen) == 0 || chosen[id] }
	start := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, s.loc)
	end := time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, s.loc)
	day := func(t time.Time) time.Time {
		t = t.In(s.loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		f, err := s.perfFacts(ctx, tx, first, last, start, end, out.FeesHidden)
		if err != nil {
			return err
		}
		out.Rows, out.Total = Performance(f, buckets, day, counts)
		if out.Conversions, err = s.conversions(ctx, tx, f.Moves, counts); err != nil {
			return err
		}
		return s.jobTimes(ctx, tx, &out, start, end, counts)
	})
	return out, tooMuch(err)
}

func (s *Service) perfFacts(ctx context.Context, tx pgx.Tx, first, last, start, end time.Time, feesHidden bool) (PerfFacts, error) {
	var f PerfFacts
	src := s.deps.Activity
	submitted, err := src.Submitted(ctx, tx, start, end)
	if err != nil {
		return f, err
	}
	emailed, err := src.CVsEmailed(ctx, tx, start, end)
	if err != nil {
		return f, err
	}
	f.CVs = append(submitted, emailed...)
	if f.Moves, err = src.CategoryMoves(ctx, tx, start, end); err != nil {
		return f, err
	}
	if f.Placements, err = s.deps.Placements.Facts(ctx, tx, first, last); err != nil {
		return f, err
	}
	if feesHidden { // the database leaves them out too
		for i := range f.Placements {
			f.Placements[i].FeeAmount, f.Placements[i].FeeCurrency = nil, nil
		}
	}
	return f, nil
}

// conversions are worked out over the chosen people's moves in the range.
func (s *Service) conversions(ctx context.Context, tx pgx.Tx, moves []CategoryMove, counts func(string) bool) ([]Conversion, error) {
	var cohort []CategoryMove
	ids := map[string]bool{}
	for _, m := range moves {
		if isPerson(m.By) && counts(m.By) {
			cohort = append(cohort, m)
			ids[m.ApplicationID] = true
		}
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	reached, err := s.deps.Applications.CategoriesReached(ctx, tx, list)
	if err != nil {
		return nil, err
	}
	cats, err := s.deps.Applications.Categories(ctx, tx)
	if err != nil {
		return nil, err
	}
	if len(cats) == 0 {
		cats = kindCategories()
	}
	order := make([]string, len(cats))
	for i, c := range cats {
		order[i] = c.Key
	}
	return Conversions(cohort, reached, order), nil
}

// jobTimes are the days from jobs being opened to their first CV and to
// their first placement in the range: the chosen people's jobs (they own).
func (s *Service) jobTimes(ctx context.Context, tx pgx.Tx, out *PerformanceReport, start, end time.Time, counts func(string) bool) error {
	cvs, err := s.deps.Applications.FirstCVs(ctx, tx, start, end)
	if err != nil {
		return err
	}
	// Jobs first placed in the range: a job filled before it and placed again
	// in it is not one.
	placed, err := s.deps.Applications.FirstPlacements(ctx, tx, start, end)
	if err != nil {
		return err
	}
	firstPlaced := map[string]time.Time{}
	for _, p := range placed {
		firstPlaced[p.JobID] = p.At
	}
	var ids []string
	for _, c := range cvs {
		ids = append(ids, c.JobID)
	}
	for id := range firstPlaced {
		ids = append(ids, id)
	}
	jobs, err := s.deps.Jobs.Jobs(ctx, tx, ids)
	if err != nil {
		return err
	}
	theirs := func(id string) (Job, bool) {
		j, ok := jobs[id]
		return j, ok && counts(idOf(j.OwnerID))
	}
	var toCV, toFill []Span
	for _, c := range cvs {
		if j, ok := theirs(c.JobID); ok {
			toCV = append(toCV, Span{JobID: c.JobID, Opened: j.Opened, Reached: c.At})
		}
	}
	for id, at := range firstPlaced {
		if j, ok := theirs(id); ok {
			toFill = append(toFill, Span{JobID: id, Opened: j.Opened, Reached: at})
		}
	}
	out.DaysToFirstCV, out.JobsWithCV = AverageDays(toCV)
	out.DaysToFill, out.JobsFilled = AverageDays(toFill)
	return nil
}
