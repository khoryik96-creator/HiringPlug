package reports

// The report calculations, as plain functions on plain values: no database,
// no HTTP.

import (
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Stages are the open pipeline stages, in order.
var Stages = []string{"applied", "screening", "submitted", "interview", "offer", "placed"}

const interviewStage = 3 // index of "interview" in Stages

// MaxDays is the longest period a report covers.
const MaxDays = 3 * 366

// Period returns the report's first and last day: the given ones, else the 90
// days up to today.
func Period(from, to *time.Time, today time.Time) (time.Time, time.Time, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	last := today
	if to != nil {
		last = *to
	}
	first := last.AddDate(0, 0, -89)
	if from != nil {
		first = *from
	}
	switch {
	case first.After(last):
		errs.Add("from", "is after the end of the period")
	case last.Sub(first) > MaxDays*24*time.Hour:
		errs.Add("from", "a report covers at most three years")
	}
	return first, last, errs
}

// App is an application as reports count it.
type App struct {
	Source string   // where it came from; "unknown" when not recorded
	Stages []string // every stage it has been at, in order
}

// Furthest is the index in Stages of the furthest open stage an application
// reached (an application added straight at a later stage passed the earlier
// ones), or -1.
func Furthest(stages []string) int {
	furthest := -1
	for _, s := range stages {
		if i := slices.Index(Stages, s); i > furthest {
			furthest = i
		}
	}
	return furthest
}

// FunnelStage is how many applications reached a stage.
type FunnelStage struct {
	Stage             string
	Reached           int
	PercentOfPrevious *int
}

// Funnel counts the applications reaching each stage, and what share of
// those at the stage before got there.
func Funnel(apps []App) []FunnelStage {
	reached := make([]int, len(Stages))
	for _, a := range apps {
		for i := 0; i <= Furthest(a.Stages); i++ {
			reached[i]++
		}
	}
	out := make([]FunnelStage, len(Stages))
	for i, s := range Stages {
		out[i] = FunnelStage{Stage: s, Reached: reached[i]}
		if i > 0 && reached[i-1] > 0 {
			pct := (reached[i]*100 + reached[i-1]/2) / reached[i-1]
			out[i].PercentOfPrevious = &pct
		}
	}
	return out
}

// SourceRow is what came of the applications from one source.
type SourceRow struct {
	Source                            string
	Applications, Interviewed, Placed int
}

// Sources counts applications, interviews and placements per source, most
// applications first.
func Sources(apps []App) []SourceRow {
	by := map[string]*SourceRow{}
	for _, a := range apps {
		r := by[a.Source]
		if r == nil {
			r = &SourceRow{Source: a.Source}
			by[a.Source] = r
		}
		r.Applications++
		f := Furthest(a.Stages)
		if f >= interviewStage {
			r.Interviewed++
		}
		if f == len(Stages)-1 {
			r.Placed++
		}
	}
	out := make([]SourceRow, 0, len(by))
	for _, r := range by {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Applications != out[j].Applications {
			return out[i].Applications > out[j].Applications
		}
		return out[i].Source < out[j].Source
	})
	return out
}

// Placement is a placement as reports count it; FeeAmount is nil when no fee
// is recorded or the user may not see fees.
type Placement struct {
	RecruiterID            *string
	Status                 string
	FeeAmount, FeeCurrency *string
}

// MoneyTotal is an amount in one currency.
type MoneyTotal struct{ Currency, Amount string }

// RecruiterRow is one recruiter's placements and fees.
type RecruiterRow struct {
	RecruiterID             *string
	Placements, FellThrough int
	Fees                    []MoneyTotal
}

// Recruiters counts placements (those that stand), fall-throughs and fees per
// recruiter credited, most placements first.
func Recruiters(ps []Placement) []RecruiterRow {
	type acc struct {
		row  RecruiterRow
		fees map[string]*big.Rat
	}
	by := map[string]*acc{}
	for _, p := range ps {
		key := ""
		if p.RecruiterID != nil {
			key = *p.RecruiterID
		}
		a := by[key]
		if a == nil {
			a = &acc{row: RecruiterRow{RecruiterID: p.RecruiterID}, fees: map[string]*big.Rat{}}
			by[key] = a
		}
		switch p.Status {
		case "active":
			a.row.Placements++
			if p.FeeAmount != nil && p.FeeCurrency != nil {
				if amount, ok := new(big.Rat).SetString(*p.FeeAmount); ok {
					if a.fees[*p.FeeCurrency] == nil {
						a.fees[*p.FeeCurrency] = new(big.Rat)
					}
					a.fees[*p.FeeCurrency].Add(a.fees[*p.FeeCurrency], amount)
				}
			}
		case "fell_through":
			a.row.FellThrough++
		}
	}
	out := make([]RecruiterRow, 0, len(by))
	for _, a := range by {
		a.row.Fees = []MoneyTotal{}
		for currency, sum := range a.fees {
			a.row.Fees = append(a.row.Fees, MoneyTotal{Currency: currency, Amount: sum.FloatString(2)})
		}
		sort.Slice(a.row.Fees, func(i, j int) bool { return a.row.Fees[i].Currency < a.row.Fees[j].Currency })
		out = append(out, a.row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Placements != out[j].Placements {
			return out[i].Placements > out[j].Placements
		}
		return idOf(out[i].RecruiterID) < idOf(out[j].RecruiterID)
	})
	return out
}

func idOf(p *string) string {
	if p == nil {
		return "~" // nobody credited: last
	}
	return *p
}

// Fill is a candidate's first placement in a job.
type Fill struct {
	JobID                           string
	JobOpened, AppCreated, PlacedAt time.Time
}

// JobFill is how long a job took to fill.
type JobFill struct {
	JobID      string
	DaysToFill int
	Placements int
}

// FillTimes are the time-to-fill figures.
type FillTimes struct {
	MedianDaysToFill, MedianDaysToPlace *int
	Jobs                                []JobFill
}

func days(from, to time.Time) int {
	d := int(to.Sub(from).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// TimeToFill works out, from first placements: per job, the days from its
// opening to its first placement; the median of those; and the median days
// from a candidate being added to the job to being placed. Slowest jobs first.
func TimeToFill(fills []Fill) FillTimes {
	type job struct {
		first time.Time
		fill  JobFill
	}
	jobs := map[string]*job{}
	var toPlace []int
	for _, f := range fills {
		toPlace = append(toPlace, days(f.AppCreated, f.PlacedAt))
		j := jobs[f.JobID]
		if j == nil {
			j = &job{first: f.PlacedAt, fill: JobFill{JobID: f.JobID}}
			jobs[f.JobID] = j
		}
		if f.PlacedAt.Before(j.first) {
			j.first = f.PlacedAt
		}
		j.fill.Placements++
		j.fill.DaysToFill = days(f.JobOpened, j.first)
	}
	out := FillTimes{Jobs: []JobFill{}, MedianDaysToPlace: median(toPlace)}
	var toFill []int
	for _, j := range jobs {
		out.Jobs = append(out.Jobs, j.fill)
		toFill = append(toFill, j.fill.DaysToFill)
	}
	sort.Slice(out.Jobs, func(i, k int) bool {
		if out.Jobs[i].DaysToFill != out.Jobs[k].DaysToFill {
			return out.Jobs[i].DaysToFill > out.Jobs[k].DaysToFill
		}
		return strings.Compare(out.Jobs[i].JobID, out.Jobs[k].JobID) < 0
	})
	out.MedianDaysToFill = median(toFill)
	return out
}

// median of whole days (the lower middle of an even count, so it is a real
// value), or nil for none.
func median(xs []int) *int {
	if len(xs) == 0 {
		return nil
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	m := s[(len(s)-1)/2]
	return &m
}
