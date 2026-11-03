package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T { return &v }

func TestFunnelAndSources(t *testing.T) {
	apps := []App{
		{Source: "jobstreet", Stages: []string{"applied", "screening", "interview", "rejected"}},
		{Source: "jobstreet", Stages: []string{"applied", "rejected"}},
		{Source: "linkedin", Stages: []string{"submitted", "interview", "offer", "placed"}}, // added at submitted
		{Source: "linkedin", Stages: []string{"applied", "screening", "interview", "offer", "placed", "offer"}},
	}
	funnel := Funnel(apps)
	reached := map[string]int{}
	for _, f := range funnel {
		reached[f.Stage] = f.Reached
	}
	// Furthest stage reached passes every earlier one: a rejected candidate who
	// was interviewed reached interview; one added at submitted passed applied.
	assert.Equal(t, map[string]int{"applied": 4, "screening": 3, "submitted": 3, "interview": 3, "offer": 2, "placed": 2}, reached)
	assert.Nil(t, funnel[0].PercentOfPrevious)
	assert.Equal(t, 75, *funnel[1].PercentOfPrevious)
	assert.Equal(t, 67, *funnel[4].PercentOfPrevious, "2 of 3 interviewed got an offer")
	assert.Equal(t, 100, *funnel[5].PercentOfPrevious, "the furthest stage counts, even if moved back since")

	assert.Equal(t, []SourceRow{
		{Source: "jobstreet", Applications: 2, Interviewed: 1, Placed: 0},
		{Source: "linkedin", Applications: 2, Interviewed: 2, Placed: 2},
	}, Sources(apps))
}

func TestRecruiters(t *testing.T) {
	rows := Recruiters([]Placement{
		{RecruiterID: ptr("usr_a"), Status: "active", FeeAmount: ptr("20400.00"), FeeCurrency: ptr("MYR")},
		{RecruiterID: ptr("usr_a"), Status: "active", FeeAmount: ptr("1000.50"), FeeCurrency: ptr("MYR")},
		{RecruiterID: ptr("usr_a"), Status: "fell_through", FeeAmount: ptr("9999"), FeeCurrency: ptr("MYR")},
		{RecruiterID: ptr("usr_b"), Status: "active"},
		{Status: "cancelled"},
	})
	assert.Len(t, rows, 3)
	assert.Equal(t, RecruiterRow{RecruiterID: ptr("usr_a"), Placements: 2, FellThrough: 1, Fees: []MoneyTotal{{Currency: "MYR", Amount: "21400.50"}}}, rows[0])
	assert.Equal(t, 1, rows[1].Placements)
	assert.Empty(t, rows[1].Fees)
	assert.Nil(t, rows[2].RecruiterID, "nobody credited comes last")
}

func TestTimeToFill(t *testing.T) {
	d := func(s string) time.Time {
		v, _ := time.Parse(time.DateOnly, s)
		return v
	}
	times := TimeToFill([]Fill{
		{JobID: "jo_1", JobOpened: d("2026-09-01"), AppCreated: d("2026-09-10"), PlacedAt: d("2026-10-01")},
		{JobID: "jo_1", JobOpened: d("2026-09-01"), AppCreated: d("2026-09-20"), PlacedAt: d("2026-10-11")},
		{JobID: "jo_2", JobOpened: d("2026-10-01"), AppCreated: d("2026-10-02"), PlacedAt: d("2026-10-11")},
	})
	assert.Equal(t, []JobFill{{JobID: "jo_1", DaysToFill: 30, Placements: 2}, {JobID: "jo_2", DaysToFill: 10, Placements: 1}}, times.Jobs, "to the first placement; slowest first")
	assert.Equal(t, 10, *times.MedianDaysToFill)
	assert.Equal(t, 21, *times.MedianDaysToPlace)
	assert.Nil(t, TimeToFill(nil).MedianDaysToFill)
}

func TestPeriod(t *testing.T) {
	today := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	from, to, errs := Period(nil, nil, today)
	assert.Empty(t, errs)
	assert.Equal(t, today, to)
	assert.Equal(t, time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC), from, "90 days including today")
	_, _, errs = Period(ptr(today.AddDate(0, 0, 1)), &today, today)
	assert.Equal(t, "from", errs[0].Field)
	_, _, errs = Period(ptr(today.AddDate(-4, 0, 0)), &today, today)
	assert.Equal(t, "from", errs[0].Field)
}
