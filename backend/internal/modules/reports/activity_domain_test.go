package reports

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPeriodOf(t *testing.T) {
	first, last := PeriodOf(Week, date("2026-11-22")) // a Sunday
	assert.Equal(t, date("2026-11-16"), first, "weeks start on Monday")
	assert.Equal(t, date("2026-11-22"), last)
	first, last = PeriodOf(Week, date("2026-11-16"))
	assert.Equal(t, date("2026-11-16"), first)
	assert.Equal(t, date("2026-11-22"), last)
	first, last = PeriodOf(Month, date("2026-02-14"))
	assert.Equal(t, date("2026-02-01"), first)
	assert.Equal(t, date("2026-02-28"), last)
}

func TestCountActivity(t *testing.T) {
	note := "note_1"
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	f := Facts{
		ScreeningCalls: []ScreeningCall{{By: "usr_a", Day: date("2026-11-18"), NoteID: &note}, {By: "usr_b", Day: date("2026-11-18")}},
		CallNotes: []CallNote{
			{ID: "note_1", Kind: "screening", By: "usr_a", At: at}, // the call's own note: not another call
			{ID: "note_2", Kind: "screening", By: "usr_a", At: at},
			{ID: "note_3", Kind: "reference", By: "usr_a", At: at},
			{ID: "note_4", Kind: "call", By: "usr_b", At: at},
		},
		ScreeningNotes: map[string]bool{"note_1": true},
		Submitted:      []CVSent{{By: "usr_a", CandidateID: "cand_1", At: at}, {By: "usr_a", CandidateID: "cand_2", At: at}},
		Emailed: []CVSent{
			{By: "usr_a", CandidateID: "cand_1", At: at.Add(3 * time.Hour)},  // the same day: once
			{By: "usr_a", CandidateID: "cand_1", At: at.Add(24 * time.Hour)}, // another day: again
			{By: "usr_b", CandidateID: "cand_1", At: at},                     // another user
		},
		Interviews: []Done{{By: "usr_b", At: at}, {By: "system", At: at}},
		Placements: []Done{{By: "usr_a", At: at}},
		CategoryMoves: []CategoryMove{
			{By: "usr_a", ApplicationID: "appl_1", CandidateID: "cand_1", Category: "interview1"},
			{By: "usr_a", ApplicationID: "appl_1", CandidateID: "cand_1", Category: "interview1"}, // moved back and in again: once
			{By: "usr_a", ApplicationID: "appl_2", CandidateID: "cand_2", Category: "interview1"},
			{By: "usr_a", ApplicationID: "appl_3", CandidateID: "cand_2", Category: "interview1"}, // the same candidate on another job: once
			{By: "usr_a", ApplicationID: "appl_1", CandidateID: "cand_1", Category: "final"},
			{By: "usr_b", ApplicationID: "appl_1", CandidateID: "cand_1", Category: "interview1"}, // another user
		},
	}
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	got := CountActivity(f, day)
	assert.Equal(t, map[string]*Counts{
		"usr_a": {Calls: 3, CVsSent: 3, Placements: 1, Categories: map[string]int{"interview1": 2, "final": 1}},
		"usr_b": {Calls: 2, CVsSent: 1, Interviews: 1, Categories: map[string]int{"interview1": 1}},
	}, got, "the app's own work counts for no one")
}

func TestTargetFor(t *testing.T) {
	ten, five, two := 10, 5, 2
	a := "usr_a"
	targets := []Target{
		{Period: Week, Goal: Goal{Calls: &ten}},
		{UserID: &a, Period: Week, Goal: Goal{Calls: &five}},
		{Period: Month, Goal: Goal{Placements: &two}},
	}
	g, ok := TargetFor(targets, "usr_a", Week)
	assert.True(t, ok)
	assert.Equal(t, 5, *g.Calls, "their own over everyone's")
	g, ok = TargetFor(targets, "usr_b", Week)
	assert.True(t, ok)
	assert.Equal(t, 10, *g.Calls)
	g, ok = TargetFor(targets, "usr_b", Month)
	assert.True(t, ok)
	assert.Nil(t, g.Calls)
	_, ok = TargetFor(targets[1:2], "usr_b", Week)
	assert.False(t, ok)
}

func TestCheckSettings(t *testing.T) {
	minus, a, gone := -1, "usr_a", "usr_gone"
	cats := map[string]bool{"interview1": true, "final": true}
	errs := CheckSettings([]Target{
		{UserID: &a, Period: Week},
		{UserID: &a, Period: Week},
		{UserID: &gone, Period: "day", Goal: Goal{CVsSent: &minus}},
		{Period: Month, Goal: Goal{Categories: map[string]int{"interview1": 4, "gone": 1, "final": -2}}},
	}, []StageWeight{{Stage: "offer", Percent: 101}, {Stage: "offer", MonthsAhead: 13}, {Stage: "applied"}}, map[string]bool{"usr_a": true}, cats)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	assert.Equal(t, map[string]bool{
		"targets.1.userId": true, "targets.2.userId": true, "targets.2.period": true, "targets.2.goal.cvsSent": true,
		"targets.3.goal.categories.gone": true, "targets.3.goal.categories.final": true,
		"stageWeights.0.percent": true, "stageWeights.1.stage": true, "stageWeights.1.monthsAhead": true, "stageWeights.2.stage": true,
	}, fields)
	assert.Empty(t, CheckSettings([]Target{{Period: Month, Goal: Goal{Categories: map[string]int{"interview1": 4}}}, {UserID: &a, Period: Month}},
		DefaultWeights, map[string]bool{"usr_a": true}, cats))
}

func TestWeights(t *testing.T) {
	got := Weights([]StageWeight{{Stage: "interview", Percent: 40, MonthsAhead: 2}})
	assert.Equal(t, []StageWeight{
		{Stage: "submitted", Percent: 10, MonthsAhead: 2}, {Stage: "interview", Percent: 40, MonthsAhead: 2},
		{Stage: "offer", Percent: 70}, {Stage: "placed", Percent: 100},
	}, got)
}
