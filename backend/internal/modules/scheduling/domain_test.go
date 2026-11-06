package scheduling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
)

var kl = func() *time.Location { l, _ := time.LoadLocation("Asia/Kuala_Lumpur"); return l }()

func at(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, kl) }

func TestCheckSettings(t *testing.T) {
	s, loc, err := CheckSettings(Settings{TimeZone: " Asia/Kuala_Lumpur ", Hours: []Hours{{2, "14:00", "17:00"}, {2, "09:00", "12:00"}}})
	require.NoError(t, err)
	assert.Equal(t, kl.String(), loc.String())
	assert.Equal(t, "09:00", s.Hours[0].Start, "sorted")

	_, _, err = CheckSettings(Settings{TimeZone: "Mars/Olympus", Hours: []Hours{{1, "10:00", "09:00"}, {1, "09:30", "11:00"}}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestCalendarURL(t *testing.T) {
	got, err := CalendarURL(" webcal://outlook.office365.com/owa/calendar/abc/reachcalendar.ics ", false)
	require.NoError(t, err)
	assert.Equal(t, "https://outlook.office365.com/owa/calendar/abc/reachcalendar.ics", got)
	for _, bad := range []string{"http://calendar.google.com/x.ics", "calendar.google.com", "https://user:pw@x.com/a.ics"} {
		_, err := CalendarURL(bad, false)
		assert.Error(t, err, bad)
	}
	_, err = CalendarURL("http://127.0.0.1:8080/x.ics", true)
	assert.NoError(t, err, "plain http for local testing only")
	assert.Equal(t, "calendar.google.com", Host("https://calendar.google.com/calendar/ical/x%40gmail.com/private-abc/basic.ics"))
}

func TestSlots(t *testing.T) {
	s := Settings{BufferMinutes: 10, Hours: []Hours{{2, "09:00", "12:00"}, {3, "14:00", "15:00"}}} // Tue, Wed
	busy := []ical.Interval{{Start: at(10, 10, 0), End: at(10, 10, 30)}}
	// Monday 9 Nov 08:00 to Wednesday 11 Nov 23:00.
	got := Slots(s, kl, 30*time.Minute, at(9, 8, 0), at(11, 23, 0), busy)
	var shown []string
	for _, g := range got {
		shown = append(shown, g.In(kl).Format("Mon 15:04"))
	}
	assert.Equal(t, []string{
		"Tue 09:00",              // 09:30 would end at 10:00, inside the 10-minute buffer before the 10:00 meeting
		"Tue 11:00", "Tue 11:30", // 10:30 starts inside the buffer after it
		"Wed 14:00", "Wed 14:30",
	}, shown)

	// Not before the earliest time.
	got = Slots(s, kl, 30*time.Minute, at(10, 11, 10), at(10, 23, 0), nil)
	require.Len(t, got, 1)
	assert.True(t, got[0].Equal(at(10, 11, 30)))

	// Short meetings every 15 minutes.
	got = Slots(Settings{Hours: []Hours{{2, "09:00", "10:00"}}}, kl, 15*time.Minute, at(10, 0, 0), at(10, 23, 0), nil)
	assert.Len(t, got, 4)
}

func TestTokens(t *testing.T) {
	a, ha, err := NewToken()
	require.NoError(t, err)
	b, _, _ := NewToken()
	assert.NotEqual(t, a, b)
	assert.Len(t, a, 43)
	assert.Equal(t, ha, HashToken(a))
	assert.NotContains(t, ha, a)
}

func TestStatusAndChanging(t *testing.T) {
	now := at(10, 9, 0)
	assert.Equal(t, "expired", Status("open", at(9, 0, 0), now))
	assert.Equal(t, "open", Status("open", at(11, 0, 0), now))
	assert.Equal(t, "booked", Status("booked", at(9, 0, 0), now), "a booking does not expire")
	soon, later := at(10, 15, 0), at(11, 15, 0)
	assert.False(t, CanChange("booked", &soon, 12*time.Hour, now))
	assert.True(t, CanChange("booked", &later, 12*time.Hour, now))
	assert.Equal(t, "Tuesday 10 November 2026, 10:00–10:30 (Asia/Kuala_Lumpur)", When(at(10, 10, 0).UTC(), 30*time.Minute, kl))
}
