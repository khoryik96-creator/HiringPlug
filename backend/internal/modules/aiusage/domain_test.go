package aiusage

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

func TestTokenCountsStayInRange(t *testing.T) {
	assert.Equal(t, int64(0), ClampTokens(-5))
	assert.Equal(t, int64(1234), ClampTokens(1234))
	assert.Equal(t, int64(MaxTokens), ClampTokens(1<<40))
	assert.Equal(t, int64(MaxTokens), ClampTokens(math.MaxInt64))
}

func TestMonthsAreTheAgencysCalendarMonths(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	require.NoError(t, err)
	// 30 Nov 17:30 UTC is already 1 Dec in Kuala Lumpur (UTC+8).
	now := time.Date(2026, 11, 30, 17, 30, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, kl), MonthStart(now, kl))
	assert.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, kl), NextMonthStart(now, kl))
	assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), MonthStart(now, time.UTC))
	assert.Equal(t, "2026-12", MonthKey(now, kl))
	assert.Equal(t, []string{"2026-12", "2026-11", "2026-10"}, MonthKeys(now, kl, 3))
	// Across a year, and from the 31st (AddDate on the 31st would skip a month).
	jan31 := time.Date(2027, 1, 31, 12, 0, 0, 0, kl)
	assert.Equal(t, []string{"2027-01", "2026-12", "2026-11", "2026-10"}, MonthKeys(jan31, kl, 4))
}

func TestDecimalsAreReadAndWrittenExactly(t *testing.T) {
	for _, c := range []struct {
		in     string
		places int
		want   int64
		ok     bool
	}{
		{"0.27", 4, 2700, true},
		{"1.1", 4, 11000, true},
		{"50", 2, 5000, true},
		{"8500.00", 2, 850000, true},
		{"0", 2, 0, true},
		{"1.234", 2, 0, false},         // too many places
		{"01.5", 2, 0, false},          // leading zero
		{"-1", 2, 0, false},            // negative
		{"1e3", 2, 0, false},           // not plain digits
		{"", 2, 0, false},              // empty
		{"1.", 2, 0, false},            // no places after the point
		{"1234567890123", 2, 0, false}, // more whole digits than allowed
	} {
		got, ok := ParseDecimal(c.in, c.places, 12)
		assert.Equal(t, c.ok, ok, c.in)
		assert.Equal(t, c.want, got, c.in)
	}
	assert.Equal(t, "8500.00", FormatDecimal(850000, 2))
	assert.Equal(t, "0.0027", FormatDecimal(27, 4))
	assert.Equal(t, "0.00", FormatDecimal(0, 2))
}

// Prices: 2.00 per million input tokens, 8.00 per million output tokens.
var twoAndEight = Prices{Set: true, Input: 20_000, Output: 80_000}

func TestCostsAreExactAndRoundedHalfUp(t *testing.T) {
	assert.Nil(t, Cost(Prices{}, Spent{Input: 1000}), "no prices: no cost")
	assert.Equal(t, "4.0000", *Cost(twoAndEight, Spent{Input: 1_000_000, Output: 250_000}))
	assert.Equal(t, "0.0000", *Cost(twoAndEight, Spent{}))
	// 25 input tokens at 2.00/M = 0.00005: half of the last place rounds
	// up; 24 (0.000048) rounds down.
	assert.Equal(t, "0.0001", *Cost(twoAndEight, Spent{Input: 25}))
	assert.Equal(t, "0.0000", *Cost(twoAndEight, Spent{Input: 24}))
	// 0.27 per million: 1,000,000,000 tokens cost 270 exactly (no float drift).
	cheap := Prices{Set: true, Input: 2700, Output: 11_000}
	assert.Equal(t, "270.0000", *Cost(cheap, Spent{Input: 1_000_000_000}))
	assert.Equal(t, "110000.0000", *Cost(cheap, Spent{Output: 100_000_000_000}))
	// Huge counts at the highest price do not overflow.
	top := Prices{Set: true, Input: 99_999_999_999_999, Output: 99_999_999_999_999}
	assert.Equal(t, "10737418239999.8926", *Cost(top, Spent{Input: MaxTokens}))
}

func TestRemainingInTokens(t *testing.T) {
	l := Limit{Kind: KindTokens, Tokens: 5000}
	assert.Equal(t, int64(5000), Remaining(l, Spent{}, 0))
	assert.Equal(t, int64(3000), Remaining(l, Spent{Input: 600, Output: 400}, 1000))
	assert.Equal(t, int64(0), Remaining(l, Spent{Input: 4000}, 1000), "exactly at the limit: nothing left")
	assert.Equal(t, int64(-500), Remaining(l, Spent{Input: 5500}, 0))
	assert.Equal(t, int64(0), Remaining(Limit{Kind: KindTokens}, Spent{}, 0), "a limit of 0 allows nothing")
}

func TestRemainingInMoney(t *testing.T) {
	// 10.00 a month at 2.00 / 8.00 per million; 4.00 spent.
	l := Limit{Kind: KindMoney, Money: 1000, Prices: twoAndEight}
	spent := Spent{Input: 1_000_000, Output: 250_000}
	// 6.00 left at the higher price (8.00 per million) is 750,000 tokens.
	assert.Equal(t, int64(750_000), Remaining(l, spent, 0))
	// A pending hold of 1,000 tokens is reserved at the higher price.
	assert.Equal(t, int64(749_000), Remaining(l, spent, 1000))
	// Rounded down: a little more spent leaves one whole token less.
	assert.Equal(t, int64(749_999), Remaining(l, Spent{Input: 1_000_001, Output: 250_000}, 0), "0.000002 more spent: one token less")
	// Exactly spent: 0; overspent: below 0, rounded towards minus infinity.
	assert.Equal(t, int64(0), Remaining(l, Spent{Input: 5_000_000}, 0))
	assert.Equal(t, int64(-1), Remaining(l, Spent{Input: 5_000_001}, 0))
	// A limit of 0.00 allows nothing.
	assert.Equal(t, int64(0), Remaining(Limit{Kind: KindMoney, Prices: twoAndEight}, Spent{}, 0))
	// Free prices: nothing costs anything.
	assert.Equal(t, int64(math.MaxInt64), Remaining(Limit{Kind: KindMoney, Money: 1000, Prices: Prices{Set: true}}, spent, 5))
	// Huge numbers clamp instead of overflowing.
	big := Limit{Kind: KindMoney, Money: math.MaxInt64, Prices: Prices{Set: true, Input: 1, Output: 1}}
	assert.Equal(t, int64(math.MaxInt64), Remaining(big, Spent{}, 0))
}

func TestUsedPercentAndReached(t *testing.T) {
	tokens := Limit{Kind: KindTokens, Tokens: 1000, WarnPercent: 80}
	assert.Equal(t, int64(0), UsedPercent(tokens, Spent{}))
	assert.Equal(t, int64(79), UsedPercent(tokens, Spent{Input: 799}))
	assert.Equal(t, int64(80), UsedPercent(tokens, Spent{Input: 700, Output: 100}))
	assert.False(t, Reached(tokens, Spent{Input: 999}))
	assert.True(t, Reached(tokens, Spent{Input: 1000}))
	assert.Equal(t, int64(150), UsedPercent(tokens, Spent{Input: 1500}))
	assert.Equal(t, int64(MaxUsedPercent), UsedPercent(Limit{Kind: KindTokens, Tokens: 1}, Spent{Input: MaxTokens}))
	assert.Equal(t, int64(100), UsedPercent(Limit{Kind: KindTokens}, Spent{}), "a limit of 0 is used up")
	assert.True(t, Reached(Limit{Kind: KindTokens}, Spent{}))

	money := Limit{Kind: KindMoney, Money: 1000, Prices: twoAndEight}
	assert.Equal(t, int64(40), UsedPercent(money, Spent{Input: 1_000_000, Output: 250_000}))
	assert.False(t, Reached(money, Spent{Input: 4_999_999}))
	assert.True(t, Reached(money, Spent{Input: 5_000_000}))
}

func TestTheLimitThatApplies(t *testing.T) {
	assert.Equal(t, Limit{Kind: KindTokens, Tokens: 2_000_000, WarnPercent: 80}, DefaultSettings().Limit(2_000_000), "the server's default")
	own := int64(5000)
	s := DefaultSettings()
	s.Tokens = &own
	assert.Equal(t, int64(5000), s.Limit(2_000_000).Tokens)
	money, in, out := int64(1000), int64(20_000), int64(80_000)
	s = Settings{Kind: KindMoney, Money: &money, PriceInput: &in, PriceOutput: &out, WarnPercent: 90}
	assert.Equal(t, Limit{Kind: KindMoney, Money: 1000, Prices: twoAndEight, WarnPercent: 90}, s.Limit(2_000_000))
	s.PriceOutput = nil
	assert.Equal(t, KindTokens, s.Limit(2_000_000).Kind, "a money limit without prices cannot apply")
	assert.False(t, s.Prices().Set)
}

func fields(err error) []string {
	if err == nil {
		return nil
	}
	e, _ := apperr.As(err)
	var out []string
	for _, f := range e.Details["fields"].(apperr.FieldErrors) {
		out = append(out, f.Field)
	}
	return out
}

func TestSettingsChecks(t *testing.T) {
	str := func(s string) *string { return &s }
	n := func(v int64) *int64 { return &v }
	ok := SettingsInput{LimitKind: KindTokens, Currency: "USD", WarnPercent: 80}
	assert.NoError(t, CheckSettings(ok))
	ok.LimitTokens = n(0)
	assert.NoError(t, CheckSettings(ok), "0 tokens stops the AI")

	money := SettingsInput{LimitKind: KindMoney, LimitMoney: str("50.00"), Currency: "MYR", WarnPercent: 80,
		PriceInputPerMillion: str("0.27"), PriceOutputPerMillion: str("1.1")}
	assert.NoError(t, CheckSettings(money))

	bad := SettingsInput{LimitKind: "dollars", LimitTokens: n(-1), Currency: "usd", WarnPercent: 100,
		LimitMoney: str("1.234"), PriceInputPerMillion: str("0.12345")}
	assert.ElementsMatch(t, []string{"limitKind", "limitTokens", "currency", "warnPercent", "limitMoney", "priceInputPerMillion"}, fields(CheckSettings(bad)))

	noPrices := SettingsInput{LimitKind: KindMoney, Currency: "USD", WarnPercent: 80}
	assert.ElementsMatch(t, []string{"limitMoney", "priceInputPerMillion", "priceOutputPerMillion"}, fields(CheckSettings(noPrices)))

	free := money
	free.PriceInputPerMillion, free.PriceOutputPerMillion = str("0"), str("0.00")
	assert.Equal(t, []string{"priceOutputPerMillion"}, fields(CheckSettings(free)))
	free.PriceOutputPerMillion = str("0.0001")
	assert.NoError(t, CheckSettings(free))
}

func TestTheReportListsEveryMonthAndSortsByTokens(t *testing.T) {
	keys := []string{"2026-11", "2026-10", "2026-09"}
	months := MonthsOf(keys, []MonthRow{
		{Month: "2026-11", Feature: FeatureAssistant, Uses: 2, Spent: Spent{Input: 100, Output: 50}},
		{Month: "2026-11", Feature: FeatureFitScore, Uses: 1, Spent: Spent{Input: 900}},
		{Month: "2026-09", Feature: FeatureCVReading, Uses: 3, Spent: Spent{Input: 30}},
		{Month: "2026-08", Feature: FeatureCVReading, Uses: 9, Spent: Spent{Input: 99}}, // outside: left out
	})
	require.Len(t, months, 3)
	assert.Equal(t, MonthUse{Month: "2026-11", Uses: 3, Spent: Spent{Input: 1000, Output: 50}, Features: []FeatureUse{
		{Feature: FeatureFitScore, Uses: 1, Spent: Spent{Input: 900}},
		{Feature: FeatureAssistant, Uses: 2, Spent: Spent{Input: 100, Output: 50}},
	}}, months[0])
	assert.Equal(t, MonthUse{Month: "2026-10", Features: []FeatureUse{}}, months[1], "a month without use is listed")
	assert.Equal(t, int64(3), months[2].Uses)

	a, b := "usr_a", "usr_b"
	people := []PersonUse{
		{UserID: &a, Name: "Zara", Spent: Spent{Input: 10}},
		{UserID: nil, Name: AutomaticName, Spent: Spent{Input: 500}},
		{UserID: &b, Name: "Adam", Spent: Spent{Input: 10}},
	}
	SortPeople(people)
	assert.Equal(t, []string{AutomaticName, "Adam", "Zara"}, []string{people[0].Name, people[1].Name, people[2].Name})
}

func TestEveryFeatureIsKnown(t *testing.T) {
	assert.Len(t, Features, 11)
	for _, f := range Features {
		assert.True(t, IsFeature(f))
	}
	assert.False(t, IsFeature("chat"))
}
