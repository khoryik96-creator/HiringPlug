package interviews

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func ptr(s string) *string { return &s }

func TestNormalize(t *testing.T) {
	d, errs := Normalize(Details{Title: "  First   interview ", Location: ptr("  "), Interviewers: []string{"u1", "u2", "u1"}})
	assert.NoError(t, errs.Err())
	assert.Equal(t, "First interview", d.Title)
	assert.Nil(t, d.Location)
	assert.Equal(t, []string{"u1", "u2"}, d.Interviewers, "each interviewer once")
	assert.Equal(t, "scheduled", d.Status)
	_, errs = Normalize(Details{})
	assert.Len(t, errs, 2) // title, interviewers
}

func TestWhoMaySeeFeedback(t *testing.T) {
	all := []FeedbackEntry{{UserID: "u1", Recommendation: "yes"}}
	panel := []string{"u1", "u2"}
	shown, hidden := VisibleFeedback(all, "u2", panel, false)
	assert.Empty(t, shown, "an interviewer who has not given feedback sees none")
	assert.Equal(t, 1, hidden)
	shown, _ = VisibleFeedback(all, "u1", panel, false)
	assert.Len(t, shown, 1, "after giving their own, they see all")
	shown, _ = VisibleFeedback(all, "u2", panel, true)
	assert.Len(t, shown, 1, "those who schedule see all")
	shown, _ = VisibleFeedback(all, "u9", panel, false)
	assert.Len(t, shown, 1, "people not on the panel are not being steered")
}

func TestFeedbackDue(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	d := Details{StartsAt: now.Add(-2 * time.Hour), Duration: 60, Interviewers: []string{"u1"}, Status: "scheduled"}
	assert.True(t, FeedbackDue("u1", d, nil, now))
	assert.False(t, FeedbackDue("u2", d, nil, now), "not on the panel")
	assert.False(t, FeedbackDue("u1", d, []FeedbackEntry{{UserID: "u1"}}, now), "already given")
	later := d
	later.StartsAt = now.Add(time.Hour)
	assert.False(t, FeedbackDue("u1", later, nil, now), "not taken place yet")
	cancelled := d
	cancelled.Status = "cancelled"
	assert.False(t, FeedbackDue("u1", cancelled, nil, now))
	assert.Contains(t, FeedbackProblem("u2", d), "Only the interview's interviewers")
	assert.Contains(t, CanSchedule("rejected"), "closed")
	assert.Empty(t, CanSchedule("screening"))
}

func TestCalendarFile(t *testing.T) {
	start := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	ics := Calendar("intv_1", 3, Details{
		Title: "First interview", StartsAt: start, Duration: 45, Status: "scheduled",
		Location: ptr("Level 5, Menara Contoh; KL"), Instructions: ptr("Bring your IC.\nAsk for Aina."),
	}, "Siti Aisyah", "Accountant", start)
	for _, want := range []string{
		"UID:intv_1@hiringplug", "SEQUENCE:3", "DTSTART:20261110T020000Z", "DTEND:20261110T024500Z",
		`LOCATION:Level 5\, Menara Contoh\; KL`, `DESCRIPTION:Bring your IC.\nAsk for Aina.`, "STATUS:CONFIRMED",
	} {
		assert.Contains(t, ics, want)
	}
	for _, line := range strings.Split(ics, "\r\n") {
		assert.LessOrEqual(t, len(line), 75, "lines are folded")
	}
}

func TestApplyOutlookChange(t *testing.T) {
	start := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	d := Details{Title: "Technical interview", StartsAt: start, Duration: 45, Mode: "video", Location: ptr("Room 4"), Interviewers: []string{"u1"}, Status: "scheduled"}
	minutes := func(n int) *int { return &n }
	_, errs := ApplyOutlookChange(d, OutlookChange{Duration: minutes(4)})
	assert.Error(t, errs.Err(), "shorter than HiringPlug schedules")
	_, errs = ApplyOutlookChange(d, OutlookChange{Duration: minutes(481)})
	assert.Error(t, errs.Err(), "longer than HiringPlug schedules")

	got, errs := ApplyOutlookChange(d, OutlookChange{Title: ptr("   ")})
	assert.NoError(t, errs.Err())
	assert.Equal(t, "Technical interview", got.Title, "an empty subject keeps the title")

	got, errs = ApplyOutlookChange(d, OutlookChange{Title: ptr(strings.Repeat("a", 150))})
	assert.NoError(t, errs.Err())
	assert.Len(t, got.Title, 100, "bounded to what an interview title holds")

	got, errs = ApplyOutlookChange(d, OutlookChange{Location: ptr("")})
	assert.NoError(t, errs.Err())
	assert.Nil(t, got.Location, "an empty place clears it")

	later := start.Add(24 * time.Hour)
	got, errs = ApplyOutlookChange(d, OutlookChange{StartsAt: &later, Duration: minutes(75), Instructions: ptr("  Bring   ID. ")})
	assert.NoError(t, errs.Err())
	assert.Equal(t, later, got.StartsAt)
	assert.Equal(t, 75, got.Duration)
	assert.Equal(t, "Bring   ID.", *got.Instructions)
	assert.Equal(t, "Room 4", *got.Location, "what Outlook did not change stays")
}
