package calendars

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRefillFollowsTheInterviewEvenFormattedAndNeverOverflows(t *testing.T) {
	s := &Service{config: Config{TimeZone: time.UTC}}
	start := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	meeting := Meeting{Start: start, End: start.Add(time.Hour), Mode: "video", Location: "https://zoom.us/j/1", Version: 3}
	html := `<p>See you <span data-field="interview.when"><b>Tue 9am</b></span> at <span data-field="interview.where">old</span>.</p>`
	got := *s.refillInterview(&html, meeting)
	assert.Equal(t, `<p>See you <span data-field="interview.when">Wednesday, 25 November 2026, 10:00 AM (GMT)</span> at <span data-field="interview.where">https://zoom.us/j/1</span>.</p>`, got)

	full := `<span data-field="interview.where">x</span>` + strings.Repeat("a", maxNotesHTML-50)
	meeting.Location = strings.Repeat("Level 12, Menara Maju ", 20)
	assert.Equal(t, full, *s.refillInterview(&full, meeting), "a refill that would not fit leaves the message as it was")
}
