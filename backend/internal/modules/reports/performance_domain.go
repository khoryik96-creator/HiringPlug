package reports

// Performance over time, as plain functions on plain values: a range of days
// split into weeks, months, quarters or years; in each, the CVs sent, the
// candidates moved into each stage category, the placements and their fees;
// how many candidates at each category went on to the next; and how long jobs
// took to their first CV and to their first placement.

import (
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Grains the performance report splits a range into (also Week and Month).
const (
	Quarter = "quarter"
	Year    = "year"
)

// MaxBuckets is the most periods one performance report has.
const MaxBuckets = 160

// Bucket is one period of a performance report (days, inclusive).
type Bucket struct{ From, To time.Time }

// bucketStart is the first day of the week, month, quarter or year day is in.
func bucketStart(grain string, day time.Time) time.Time {
	switch grain {
	case Week:
		first, _ := PeriodOf(Week, day)
		return first
	case Quarter:
		return time.Date(day.Year(), (day.Month()-1)/3*3+1, 1, 0, 0, 0, 0, time.UTC)
	case Year:
		return time.Date(day.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
}

func bucketEnd(grain string, start time.Time) time.Time {
	switch grain {
	case Week:
		return start.AddDate(0, 0, 6)
	case Quarter:
		return start.AddDate(0, 3, -1)
	case Year:
		return start.AddDate(1, 0, -1)
	default:
		return start.AddDate(0, 1, -1)
	}
}

// Buckets splits first..last into the grain's periods, the first and last
// cut to the range.
func Buckets(grain string, first, last time.Time) ([]Bucket, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	if grain != Week && grain != Month && grain != Quarter && grain != Year {
		errs.Add("grain", "must be week, month, quarter or year")
		return nil, errs
	}
	if first.After(last) {
		errs.Add("from", "is after the end of the period")
		return nil, errs
	}
	var out []Bucket
	for start := bucketStart(grain, first); !start.After(last); start = bucketEnd(grain, start).AddDate(0, 0, 1) {
		if len(out) == MaxBuckets {
			errs.Add("grain", "too many periods: choose a longer one or a shorter range")
			return nil, errs
		}
		b := Bucket{From: start, To: bucketEnd(grain, start)}
		if b.From.Before(first) {
			b.From = first
		}
		if b.To.After(last) {
			b.To = last
		}
		out = append(out, b)
	}
	return out, errs
}

// PerfFacts are what happened in a range: CVs sent, moves into categories
// (with when), and placements starting.
type PerfFacts struct {
	CVs        []CVSent
	Moves      []CategoryMove
	Placements []Placement
}

// PerfRow is one period's figures (or the whole range's).
type PerfRow struct {
	From, To   time.Time
	CVsSent    int
	Categories map[string]int
	Placements int
	Fees       []MoneyTotal
}

// Performance counts each period's figures and the whole range's, for the
// people who counts (by user id; "" is no one credited). A CV counts once per
// person, candidate and day; a candidate once per person and category in a
// period (and in the range). Placements that stand count, with their fees.
func Performance(f PerfFacts, buckets []Bucket, day func(time.Time) time.Time, counts func(string) bool) ([]PerfRow, PerfRow) {
	rows := make([]PerfRow, len(buckets))
	for i, b := range buckets {
		rows[i] = PerfRow{From: b.From, To: b.To, Categories: map[string]int{}}
	}
	total := PerfRow{Categories: map[string]int{}}
	if len(buckets) > 0 {
		total.From, total.To = buckets[0].From, buckets[len(buckets)-1].To
	}
	in := func(d time.Time) int {
		i := sort.Search(len(buckets), func(i int) bool { return !buckets[i].To.Before(d) })
		if i < len(buckets) && !d.Before(buckets[i].From) {
			return i
		}
		return -1
	}
	seen := map[string]bool{}
	for _, cv := range f.CVs {
		d := day(cv.At)
		key := cv.By + "|" + cv.CandidateID + "|" + d.Format(time.DateOnly)
		if i := in(d); i >= 0 && counts(cv.By) && !seen[key] {
			seen[key] = true
			rows[i].CVsSent++
			total.CVsSent++
		}
	}
	for _, m := range f.Moves {
		i := in(day(m.At))
		if i < 0 || !counts(m.By) {
			continue
		}
		key := m.By + "|" + m.ApplicationID + "|" + m.Category
		if bucketKey := key + "|" + rows[i].From.Format(time.DateOnly); !seen[bucketKey] {
			seen[bucketKey] = true
			rows[i].Categories[m.Category]++
		}
		if !seen[key] {
			seen[key] = true
			total.Categories[m.Category]++
		}
	}
	fees := make([]map[string]*big.Rat, len(rows))
	all := map[string]*big.Rat{}
	for _, p := range f.Placements {
		i := in(p.StartDate)
		if i < 0 || p.Status != "active" || !counts(idOf(p.RecruiterID)) {
			continue
		}
		rows[i].Placements++
		total.Placements++
		if p.FeeAmount == nil || p.FeeCurrency == nil {
			continue
		}
		amount, ok := new(big.Rat).SetString(*p.FeeAmount)
		if !ok {
			continue
		}
		if fees[i] == nil {
			fees[i] = map[string]*big.Rat{}
		}
		for _, sums := range []map[string]*big.Rat{fees[i], all} {
			if sums[*p.FeeCurrency] == nil {
				sums[*p.FeeCurrency] = new(big.Rat)
			}
			sums[*p.FeeCurrency].Add(sums[*p.FeeCurrency], amount)
		}
	}
	for i := range rows {
		rows[i].Fees = moneyTotals(fees[i])
	}
	total.Fees = moneyTotals(all)
	return rows, total
}

func moneyTotals(sums map[string]*big.Rat) []MoneyTotal {
	out := []MoneyTotal{}
	for currency, sum := range sums {
		out = append(out, MoneyTotal{Currency: currency, Amount: sum.FloatString(2)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}

// Conversion is, of the candidates moved into one category in the range, how
// many reached the next (or went further) by now.
type Conversion struct {
	From, To         string
	Entered, Reached int
}

// Conversions works out, for each category after the first (in the company's
// order), how many candidates moved into the one before it in the range went
// on to reach it or further. reached gives every category each application
// has been in, ever.
func Conversions(cohort []CategoryMove, reached map[string][]string, order []string) []Conversion {
	pos := map[string]int{}
	for i, key := range order {
		pos[key] = i
	}
	furthest := map[string]int{}
	for app, cats := range reached {
		furthest[app] = -1
		for _, c := range cats {
			if p, ok := pos[c]; ok && p > furthest[app] {
				furthest[app] = p
			}
		}
	}
	entered := map[string]map[string]bool{} // category -> applications
	for _, m := range cohort {
		if entered[m.Category] == nil {
			entered[m.Category] = map[string]bool{}
		}
		entered[m.Category][m.ApplicationID] = true
	}
	var out []Conversion
	for i := 1; i < len(order); i++ {
		c := Conversion{From: order[i-1], To: order[i]}
		for app := range entered[order[i-1]] {
			c.Entered++
			if f, ok := furthest[app]; ok && f >= i {
				c.Reached++
			}
		}
		out = append(out, c)
	}
	return out
}

// Span is a job's time from being opened to something (its first CV sent,
// or its first placement).
type Span struct {
	JobID           string
	Opened, Reached time.Time
}

// AverageDays is the average of the spans in days (one decimal), and how many
// jobs it is over; nil when there are none.
func AverageDays(spans []Span) (*float64, int) {
	if len(spans) == 0 {
		return nil, 0
	}
	sum := 0.0
	for _, s := range spans {
		sum += math.Max(0, s.Reached.Sub(s.Opened).Hours()/24)
	}
	avg := math.Round(sum/float64(len(spans))*10) / 10
	return &avg, len(spans)
}
