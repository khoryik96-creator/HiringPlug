package reports

// The revenue forecast, as plain functions on plain values: the fee a job
// would bring (the client's rate for its salary budget), each application's
// fee weighted by its stage, and the totals per month and per recruiter.

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

// Budget is a job's salary budget. Max is nil when only one figure is set.
type Budget struct {
	Min              string
	Max              *string
	Currency, Period string // period: hour, day, month, year
}

// Rate is one of a client's rates: for annual salaries from From (inclusive)
// below Below (exclusive), nil limits open; Percent of the annual salary.
type Rate struct{ From, Below, Percent *string }

// Terms are a client's terms of business: their rates, with salary limits in
// Currency. Set is false when the client has none.
type Terms struct {
	Set      bool
	Currency string
	Rates    []Rate
}

// Why a fee could not be worked out.
const (
	GapNoBudget = "no_salary_budget"
	GapNoTerms  = "no_terms"
	GapNoRate   = "no_rate"
)

func rat(s *string) *big.Rat {
	if s == nil {
		return nil
	}
	r, ok := new(big.Rat).SetString(*s)
	if !ok {
		return nil
	}
	return r
}

// ExpectedFee is the fee a placement in a job would bring: the client's rate
// for the budget's annual salary (the middle of its range; a monthly salary
// counts 12 times) as a percentage of it, in the budget's currency. A budget
// in another currency than the rates' limits takes the client's only rate,
// when that has no limits. Else gap says why there is none.
func ExpectedFee(b *Budget, t Terms) (fee *big.Rat, currency, gap string) {
	if b == nil || (b.Period != "month" && b.Period != "year") || rat(&b.Min) == nil {
		return nil, "", GapNoBudget
	}
	hasRate := false
	for _, r := range t.Rates {
		hasRate = hasRate || rat(r.Percent) != nil
	}
	if !t.Set || !hasRate {
		return nil, "", GapNoTerms
	}
	salary := rat(&b.Min)
	if top := rat(b.Max); top != nil {
		salary = new(big.Rat).Quo(new(big.Rat).Add(salary, top), big.NewRat(2, 1))
	}
	if b.Period == "month" {
		salary.Mul(salary, big.NewRat(12, 1))
	}
	var pick *Rate
	termsCurrency := t.Currency
	if termsCurrency == "" {
		termsCurrency = "MYR"
	}
	if strings.EqualFold(b.Currency, termsCurrency) {
		for i, r := range t.Rates {
			from, below := rat(r.From), rat(r.Below)
			if (from == nil || salary.Cmp(from) >= 0) && (below == nil || salary.Cmp(below) < 0) {
				pick = &t.Rates[i]
				break
			}
		}
	} else if len(t.Rates) == 1 && t.Rates[0].From == nil && t.Rates[0].Below == nil {
		pick = &t.Rates[0]
	}
	if pick == nil || rat(pick.Percent) == nil {
		return nil, "", GapNoRate
	}
	fee = new(big.Rat).Mul(salary, rat(pick.Percent))
	return fee.Quo(fee, big.NewRat(100, 1)), strings.ToUpper(b.Currency), ""
}

// Line is an application counted in the forecast.
type Line struct {
	ApplicationID, CandidateName, JobTitle, CompanyName, Stage string
	RecruiterID                                                *string
	Month                                                      string // 2026-11
	Currency                                                   string
	Fee, Expected                                              *big.Rat
	Percent                                                    int
	FromPlacement                                              bool // else from the client's terms
}

// Gap is an application whose fee could not be worked out.
type Gap struct {
	ApplicationID, CandidateName, JobTitle, Stage, Reason string
}

// ForecastMonth is the month a fee is expected in: a placement's start date
// (this month when it has passed), else this month plus the stage's months
// ahead.
func ForecastMonth(today time.Time, start *time.Time, monthsAhead int) string {
	this := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	if start != nil {
		m := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		if m.Before(this) {
			m = this
		}
		return m.Format("2006-01")
	}
	return this.AddDate(0, monthsAhead, 0).Format("2006-01")
}

// Weighted is a fee times a stage's percentage.
func Weighted(fee *big.Rat, percent int) *big.Rat {
	out := new(big.Rat).Mul(fee, big.NewRat(int64(percent), 1))
	return out.Quo(out, big.NewRat(100, 1))
}

// Total is what is expected (weighted) and the fees before weighting, in one
// currency.
type Total struct {
	Currency       string
	Expected, Full *big.Rat
}

// Group is the totals of a month or a recruiter.
type Group struct {
	Key    string // the month, or the recruiter's ID ("" for none)
	Totals []Total
}

func add(totals []Total, l Line) []Total {
	for i := range totals {
		if totals[i].Currency == l.Currency {
			totals[i].Expected.Add(totals[i].Expected, l.Expected)
			totals[i].Full.Add(totals[i].Full, l.Fee)
			return totals
		}
	}
	return append(totals, Total{Currency: l.Currency, Expected: new(big.Rat).Set(l.Expected), Full: new(big.Rat).Set(l.Fee)})
}

// ByMonth adds the lines up per month, in order, each month's currencies
// most expected first.
func ByMonth(lines []Line) []Group {
	groups := groupBy(lines, func(l Line) string { return l.Month })
	sort.Slice(groups, func(i, j int) bool { return groups[i].Key < groups[j].Key })
	return groups
}

// ByRecruiter adds the lines up per recruiter, most expected first. Amounts
// in different currencies are not compared: recruiters are ranked by what
// they expect in the forecast's main currency (the one most lines are in),
// those with none in it after, by name key.
func ByRecruiter(lines []Line) []Group {
	groups := groupBy(lines, func(l Line) string {
		if l.RecruiterID == nil {
			return ""
		}
		return *l.RecruiterID
	})
	main := mainCurrency(lines)
	in := func(g Group) *big.Rat {
		for _, t := range g.Totals {
			if t.Currency == main {
				return t.Expected
			}
		}
		return nil
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := in(groups[i]), in(groups[j])
		switch {
		case a != nil && b == nil:
			return true
		case a == nil && b != nil:
			return false
		case a != nil:
			if c := a.Cmp(b); c != 0 {
				return c > 0
			}
		}
		return groups[i].Key < groups[j].Key
	})
	return groups
}

// mainCurrency is the currency most lines are in (the first such, by name).
func mainCurrency(lines []Line) string {
	count := map[string]int{}
	main := ""
	for _, l := range lines {
		count[l.Currency]++
		if n := count[l.Currency]; n > count[main] || (n == count[main] && l.Currency < main) {
			main = l.Currency
		}
	}
	return main
}

func groupBy(lines []Line, key func(Line) string) []Group {
	index := map[string]int{}
	var out []Group
	for _, l := range lines {
		k := key(l)
		i, ok := index[k]
		if !ok {
			i, index[k] = len(out), len(out)
			out = append(out, Group{Key: k})
		}
		out[i].Totals = add(out[i].Totals, l)
	}
	for _, g := range out {
		sort.SliceStable(g.Totals, func(i, j int) bool {
			if c := g.Totals[i].Expected.Cmp(g.Totals[j].Expected); c != 0 {
				return c > 0
			}
			return g.Totals[i].Currency < g.Totals[j].Currency
		})
	}
	return out
}

// SortLines puts the most expected first (then by candidate).
func SortLines(lines []Line) {
	sort.SliceStable(lines, func(i, j int) bool {
		if c := lines[i].Expected.Cmp(lines[j].Expected); c != 0 {
			return c > 0
		}
		return lines[i].CandidateName < lines[j].CandidateName
	})
}

// Money writes an amount with two decimals.
func Money(r *big.Rat) string { return r.FloatString(2) }
