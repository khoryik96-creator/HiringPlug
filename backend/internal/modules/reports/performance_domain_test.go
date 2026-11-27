package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuckets(t *testing.T) {
	b, errs := Buckets(Quarter, date("2026-02-10"), date("2026-11-27"))
	require.Empty(t, errs)
	require.Len(t, b, 4)
	assert.Equal(t, Bucket{From: date("2026-02-10"), To: date("2026-03-31")}, b[0], "the first cut to the range")
	assert.Equal(t, Bucket{From: date("2026-04-01"), To: date("2026-06-30")}, b[1])
	assert.Equal(t, Bucket{From: date("2026-10-01"), To: date("2026-11-27")}, b[3], "the last cut to today")

	b, _ = Buckets(Week, date("2026-11-18"), date("2026-11-27"))
	assert.Equal(t, []Bucket{{date("2026-11-18"), date("2026-11-22")}, {date("2026-11-23"), date("2026-11-27")}}, b, "weeks run Monday to Sunday")
	b, _ = Buckets(Year, date("2024-06-01"), date("2026-01-15"))
	assert.Len(t, b, 3)
	b, _ = Buckets(Month, date("2026-01-01"), date("2026-12-31"))
	assert.Len(t, b, 12)

	_, errs = Buckets(Week, date("2020-01-01"), date("2026-11-27"))
	assert.NotEmpty(t, errs, "too many weeks")
	_, errs = Buckets("day", date("2026-01-01"), date("2026-01-02"))
	assert.NotEmpty(t, errs)
}

func TestPerformance(t *testing.T) {
	at := func(s string) time.Time { return date(s).Add(9 * time.Hour) }
	fee := func(amount, currency string) (*string, *string) { return &amount, &currency }
	a, b := "usr_a", "usr_b"
	f := PerfFacts{
		CVs: []CVSent{
			{By: a, CandidateID: "c1", At: at("2026-01-05")}, {By: a, CandidateID: "c1", At: at("2026-01-05")}, // the same day: once
			{By: a, CandidateID: "c2", At: at("2026-02-02")}, {By: b, CandidateID: "c3", At: at("2026-02-02")},
		},
		Moves: []CategoryMove{
			{By: a, ApplicationID: "app1", Category: "interview", At: at("2026-01-10")},
			{By: a, ApplicationID: "app1", Category: "interview", At: at("2026-02-10")}, // again, in another month
			{By: b, ApplicationID: "app2", Category: "interview", At: at("2026-02-11")},
		},
	}
	f1a, f1c := fee("6000.50", "MYR")
	f2a, f2c := fee("4000", "MYR")
	f3a, f3c := fee("1000", "SGD")
	f.Placements = []Placement{
		{RecruiterID: &a, Status: "active", FeeAmount: f1a, FeeCurrency: f1c, StartDate: date("2026-01-20")},
		{RecruiterID: &a, Status: "active", FeeAmount: f2a, FeeCurrency: f2c, StartDate: date("2026-02-20")},
		{RecruiterID: &b, Status: "active", FeeAmount: f3a, FeeCurrency: f3c, StartDate: date("2026-02-21")},
		{RecruiterID: &a, Status: "fell_through", FeeAmount: f2a, FeeCurrency: f2c, StartDate: date("2026-02-22")},
	}
	buckets, _ := Buckets(Month, date("2026-01-01"), date("2026-02-28"))
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }

	rows, total := Performance(f, buckets, day, func(string) bool { return true })
	assert.Equal(t, 1, rows[0].CVsSent)
	assert.Equal(t, 2, rows[1].CVsSent)
	assert.Equal(t, map[string]int{"interview": 1}, rows[0].Categories)
	assert.Equal(t, map[string]int{"interview": 2}, rows[1].Categories, "moved in again in February counts in February")
	assert.Equal(t, map[string]int{"interview": 2}, total.Categories, "but once in the range")
	assert.Equal(t, []MoneyTotal{{Currency: "MYR", Amount: "6000.50"}}, rows[0].Fees)
	assert.Equal(t, []MoneyTotal{{Currency: "MYR", Amount: "4000.00"}, {Currency: "SGD", Amount: "1000.00"}}, rows[1].Fees, "fell through: left out")
	assert.Equal(t, 3, total.Placements)
	assert.Equal(t, []MoneyTotal{{Currency: "MYR", Amount: "10000.50"}, {Currency: "SGD", Amount: "1000.00"}}, total.Fees)

	_, onlyB := Performance(f, buckets, day, func(id string) bool { return id == b })
	assert.Equal(t, 1, onlyB.CVsSent)
	assert.Equal(t, 1, onlyB.Placements)
	assert.Equal(t, map[string]int{"interview": 1}, onlyB.Categories)
}

func TestConversions(t *testing.T) {
	order := []string{"submitted", "interview", "interview_final", "offer", "placed"}
	cohort := []CategoryMove{
		{ApplicationID: "app1", Category: "submitted"}, {ApplicationID: "app2", Category: "submitted"},
		{ApplicationID: "app3", Category: "submitted"}, {ApplicationID: "app4", Category: "offer"},
	}
	reached := map[string][]string{
		"app1": {"submitted", "interview"},
		"app2": {"submitted", "interview", "interview_final", "offer"}, // went further: counts for every step
		"app3": {"submitted"},
		"app4": {"offer", "placed"},
	}
	got := Conversions(cohort, reached, order)
	assert.Equal(t, []Conversion{
		{From: "submitted", To: "interview", Entered: 3, Reached: 2},
		{From: "interview", To: "interview_final", Entered: 0, Reached: 0},
		{From: "interview_final", To: "offer", Entered: 0, Reached: 0},
		{From: "offer", To: "placed", Entered: 1, Reached: 1},
	}, got)
}

func TestAverageDays(t *testing.T) {
	avg, n := AverageDays([]Span{
		{Opened: date("2026-01-01"), Reached: date("2026-01-04")},
		{Opened: date("2026-01-01"), Reached: date("2026-01-11")},
	})
	require.NotNil(t, avg)
	assert.InDelta(t, 6.5, *avg, 0.001)
	assert.Equal(t, 2, n)
	avg, n = AverageDays(nil)
	assert.Nil(t, avg)
	assert.Zero(t, n)
}
