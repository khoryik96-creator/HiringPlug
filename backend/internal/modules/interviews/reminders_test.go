package interviews

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReminderTime(t *testing.T) {
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	at, ok := ReminderTime(now.Add(72*time.Hour), now)
	assert.True(t, ok)
	assert.Equal(t, now.Add(48*time.Hour), at, "a day before")
	at, ok = ReminderTime(now.Add(5*time.Hour), now)
	assert.True(t, ok)
	assert.Equal(t, now, at, "less than a day away: straight away")
	_, ok = ReminderTime(now.Add(30*time.Minute), now)
	assert.False(t, ok, "within the hour: too late to help")
}

func TestReminderText(t *testing.T) {
	kl, err := time.LoadLocation("Asia/Kuala_Lumpur")
	assert.NoError(t, err)
	link := "https://meet.example/abc"
	subject, body := ReminderText(
		Details{Title: "First interview", StartsAt: time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC), Duration: 45, Mode: "video", Location: &link},
		Application{CandidateID: "cand_1", CandidateName: "Siti Aisyah", JobTitle: "Accountant"},
		[]string{"Aina", "Wei"}, kl, "https://hire.example.com",
	)
	assert.Equal(t, "Reminder: First interview with Siti Aisyah, Tuesday, 10 November 2026, 10:00 AM (GMT+8)", subject)
	assert.Contains(t, body, "You are interviewing Siti Aisyah for Accountant.")
	assert.Contains(t, body, "(45 minutes)\nHow: Video call\nWhere: https://meet.example/abc\nInterviewers: Aina, Wei")
	assert.Contains(t, body, "https://hire.example.com/#/candidates/cand_1")
}
