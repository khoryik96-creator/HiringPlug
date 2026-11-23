package reports

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func s(v string) *string { return &v }

func TestExpectedFee(t *testing.T) {
	tiers := Terms{Set: true, Currency: "MYR", Rates: []Rate{
		{Below: s("60000"), Percent: s("15")},
		{From: s("60000"), Percent: s("20")},
	}}
	cases := []struct {
		name     string
		budget   *Budget
		terms    Terms
		fee, cur string
		gap      string
	}{
		{"monthly: 12 times, the middle of the range", &Budget{Min: "4000", Max: s("6000"), Currency: "MYR", Period: "month"}, tiers, "12000.00", "MYR", ""},
		{"the rate for the salary", &Budget{Min: "100000", Currency: "myr", Period: "year"}, tiers, "20000.00", "MYR", ""},
		{"no budget", nil, tiers, "", "", GapNoBudget},
		{"a budget per day", &Budget{Min: "300", Currency: "MYR", Period: "day"}, tiers, "", "", GapNoBudget},
		{"no terms", &Budget{Min: "5000", Currency: "MYR", Period: "month"}, Terms{}, "", "", GapNoTerms},
		{"rates without a fee", &Budget{Min: "5000", Currency: "MYR", Period: "month"}, Terms{Set: true, Rates: []Rate{{Below: s("1")}}}, "", "", GapNoTerms},
		{"another currency, several rates", &Budget{Min: "5000", Currency: "SGD", Period: "month"}, tiers, "", "", GapNoRate},
		{"another currency, one open rate", &Budget{Min: "5000", Currency: "SGD", Period: "month"},
			Terms{Set: true, Currency: "MYR", Rates: []Rate{{Percent: s("18")}}}, "10800.00", "SGD", ""},
		{"no rate fits", &Budget{Min: "5000", Currency: "MYR", Period: "month"},
			Terms{Set: true, Rates: []Rate{{From: s("100000"), Percent: s("25")}}}, "", "", GapNoRate},
	}
	for _, c := range cases {
		fee, cur, gap := ExpectedFee(c.budget, c.terms)
		assert.Equal(t, c.gap, gap, c.name)
		assert.Equal(t, c.cur, cur, c.name)
		if c.fee != "" {
			assert.Equal(t, c.fee, Money(fee), c.name)
		} else {
			assert.Nil(t, fee, c.name)
		}
	}
}

func TestForecastMonth(t *testing.T) {
	today := date("2026-11-22")
	assert.Equal(t, "2026-11", ForecastMonth(today, nil, 0))
	assert.Equal(t, "2027-01", ForecastMonth(today, nil, 2))
	start := date("2027-02-03")
	assert.Equal(t, "2027-02", ForecastMonth(today, &start, 0), "a placement: its start date")
	past := date("2026-09-01")
	assert.Equal(t, "2026-11", ForecastMonth(today, &past, 0), "started, not yet invoiced: expected now")
}

func TestForecastTotals(t *testing.T) {
	r := func(v string) *big.Rat { x, _ := new(big.Rat).SetString(v); return x }
	a, b := "usr_a", "usr_b"
	lines := []Line{
		{CandidateName: "Siti", RecruiterID: &a, Month: "2026-12", Currency: "MYR", Fee: r("10000"), Expected: Weighted(r("10000"), 30)},
		{CandidateName: "Wei", RecruiterID: &b, Month: "2026-11", Currency: "MYR", Fee: r("20000"), Expected: Weighted(r("20000"), 70)},
		{CandidateName: "Aina", RecruiterID: &a, Month: "2026-11", Currency: "SGD", Fee: r("5000"), Expected: Weighted(r("5000"), 100)},
		{CandidateName: "Ali", Month: "2026-11", Currency: "MYR", Fee: r("1000"), Expected: Weighted(r("1000"), 10)},
	}
	months := ByMonth(lines)
	assert.Equal(t, "2026-11", months[0].Key)
	assert.Equal(t, "MYR", months[0].Totals[0].Currency, "most expected first")
	assert.Equal(t, "14100.00", Money(months[0].Totals[0].Expected))
	assert.Equal(t, "21000.00", Money(months[0].Totals[0].Full))
	assert.Equal(t, "5000.00", Money(months[0].Totals[1].Expected))
	assert.Equal(t, "2026-12", months[1].Key)
	recruiters := ByRecruiter(lines)
	assert.Equal(t, []string{"usr_b", "usr_a", ""}, []string{recruiters[0].Key, recruiters[1].Key, recruiters[2].Key})
	SortLines(lines)
	assert.Equal(t, "Wei", lines[0].CandidateName)
	assert.Equal(t, "Ali", lines[3].CandidateName)
}

// Recruiters are ranked by what they expect in the main currency, never by
// comparing amounts in different currencies.
func TestRecruitersRankedInOneCurrency(t *testing.T) {
	r := func(v string) *big.Rat { x, _ := new(big.Rat).SetString(v); return x }
	a, b, c := "usr_a", "usr_b", "usr_c"
	lines := []Line{
		{RecruiterID: &a, Currency: "USD", Fee: r("90000"), Expected: r("90000")}, // a big number, in another currency
		{RecruiterID: &a, Currency: "MYR", Fee: r("1000"), Expected: r("1000")},
		{RecruiterID: &b, Currency: "MYR", Fee: r("30000"), Expected: r("30000")},
		{RecruiterID: &c, Currency: "SGD", Fee: r("50000"), Expected: r("50000")},
	}
	got := ByRecruiter(lines)
	assert.Equal(t, []string{"usr_b", "usr_a", "usr_c"}, []string{got[0].Key, got[1].Key, got[2].Key},
		"MYR is the main currency: b expects more of it than a; c has none")
}
