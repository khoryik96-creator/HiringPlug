package ical_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
)

var kl = func() *time.Location { l, _ := time.LoadLocation("Asia/Kuala_Lumpur"); return l }()

// As Google Calendar's secret iCal address serves it.
const google = "BEGIN:VCALENDAR\r\nPRODID:-//Google Inc//Google Calendar 70.9054//EN\r\nVERSION:2.0\r\n" +
	"X-WR-TIMEZONE:Asia/Kuala_Lumpur\r\n" +
	// A one-off meeting in UTC.
	"BEGIN:VEVENT\r\nDTSTART:20261110T020000Z\r\nDTEND:20261110T030000Z\r\nUID:one@google.com\r\nSUMMARY:Client call\r\nTRANSP:OPAQUE\r\nEND:VEVENT\r\n" +
	// A weekly stand-up on Mondays and Wednesdays at 09:30 KL, one skipped, one moved.
	"BEGIN:VEVENT\r\nDTSTART;TZID=Asia/Kuala_Lumpur:20261102T093000\r\nDTEND;TZID=Asia/Kuala_Lumpur:20261102T100000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20261130T000000Z\r\nEXDATE;TZID=Asia/Kuala_Lumpur:20261111T093000\r\n" +
	"UID:standup@google.com\r\nSUMMARY:Stand-up with a very long title that goes on and on so that Google\r\n  folds it\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;TZID=Asia/Kuala_Lumpur:20261116T140000\r\nDTEND;TZID=Asia/Kuala_Lumpur:20261116T143000\r\n" +
	"RECURRENCE-ID;TZID=Asia/Kuala_Lumpur:20261116T093000\r\nUID:standup@google.com\r\nEND:VEVENT\r\n" +
	// Free, cancelled, and an all-day event marked free (Google's default).
	"BEGIN:VEVENT\r\nDTSTART:20261112T010000Z\r\nDTEND:20261112T020000Z\r\nTRANSP:TRANSPARENT\r\nUID:free@google.com\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART:20261113T010000Z\r\nDTEND:20261113T020000Z\r\nSTATUS:CANCELLED\r\nUID:gone@google.com\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20261114\r\nDTEND;VALUE=DATE:20261115\r\nTRANSP:TRANSPARENT\r\nUID:bday@google.com\r\nEND:VEVENT\r\n" +
	// An all-day event that blocks the day.
	"BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20261120\r\nDTEND;VALUE=DATE:20261121\r\nUID:leave@google.com\r\nSUMMARY:Leave\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

// As an Outlook published calendar serves it: Windows zone names.
const outlook = "BEGIN:VCALENDAR\r\nMETHOD:PUBLISH\r\nPRODID:Microsoft Exchange Server 2010\r\nVERSION:2.0\r\n" +
	"BEGIN:VTIMEZONE\r\nTZID:Malay Peninsula Standard Time\r\nBEGIN:STANDARD\r\nDTSTART:16010101T000000\r\nTZOFFSETFROM:+0800\r\nTZOFFSETTO:+0800\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
	"BEGIN:VTIMEZONE\r\nTZID:Made Up Zone\r\nBEGIN:STANDARD\r\nDTSTART:16010101T000000\r\nTZOFFSETFROM:+0530\r\nTZOFFSETTO:+0530\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
	"BEGIN:VEVENT\r\nUID:o1\r\nDTSTART;TZID=Malay Peninsula Standard Time:20261110T150000\r\nDTEND;TZID=Malay Peninsula Standard Time:20261110T160000\r\n" +
	"X-MICROSOFT-CDO-BUSYSTATUS:BUSY\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:o2\r\nDTSTART;TZID=Malay Peninsula Standard Time:20261110T170000\r\nDURATION:PT30M\r\nX-MICROSOFT-CDO-BUSYSTATUS:FREE\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:o3\r\nDTSTART;TZID=Made Up Zone:20261111T100000\r\nDURATION:PT1H\r\nEND:VEVENT\r\n" +
	"END:VCALENDAR\r\n"

func at(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, kl) }

func TestBusyFromAGoogleCalendar(t *testing.T) {
	got, err := ical.Busy([]byte(google), at(9, 0, 0), at(23, 0, 0), kl)
	require.NoError(t, err)
	want := []ical.Interval{
		{Start: at(9, 9, 30), End: at(9, 10, 0)},   // stand-up, Monday
		{Start: at(10, 10, 0), End: at(10, 11, 0)}, // client call (02:00 UTC)
		// Wednesday the 11th was skipped (EXDATE).
		{Start: at(16, 14, 0), End: at(16, 14, 30)}, // Monday the 16th, moved to 14:00
		{Start: at(18, 9, 30), End: at(18, 10, 0)},
		{Start: at(20, 0, 0), End: at(21, 0, 0)}, // leave, all day
	}
	assert.Equal(t, len(want), len(got), "%v", got)
	for i := range want {
		if i < len(got) {
			assert.True(t, want[i].Start.Equal(got[i].Start) && want[i].End.Equal(got[i].End), "%d: want %v, got %v", i, want[i], got[i])
		}
	}
}

func TestBusyFromAnOutlookCalendar(t *testing.T) {
	got, err := ical.Busy([]byte(outlook), at(1, 0, 0), at(30, 0, 0), kl)
	require.NoError(t, err)
	require.Len(t, got, 2, "the free one is left out")
	assert.True(t, got[0].Start.Equal(at(10, 15, 0)), "a Windows zone name: %v", got[0].Start)
	assert.True(t, got[1].Start.Equal(time.Date(2026, 11, 11, 10, 0, 0, 0, time.FixedZone("x", 5*3600+1800))), "an unknown zone uses its VTIMEZONE: %v", got[1].Start)
	assert.Equal(t, time.Hour, got[1].End.Sub(got[1].Start))
}

func TestBusyRefusesWhatIsNotACalendar(t *testing.T) {
	_, err := ical.Busy([]byte("<html>Sign in</html>"), at(1, 0, 0), at(2, 0, 0), kl)
	assert.ErrorIs(t, err, ical.ErrNotCalendar)
}

func TestAnInvitation(t *testing.T) {
	in := ical.Invite{
		Method: "REQUEST", UID: "bkl_1@hiringplug", Sequence: 2,
		Organizer: ical.Person{Name: "Contoh Recruitment", Email: "no-reply@contoh.example"},
		Attendees: []ical.Person{{Name: "Aina", Email: "aina@contoh.example"}, {Name: "Siti, A.", Email: "siti@example.com"}},
		Start:     at(10, 10, 0), End: at(10, 10, 30), Summary: "Call with Aina; Contoh", Location: "https://meet.google.com/abc-defg-hij",
	}
	s := in.Render(at(6, 9, 0))
	for _, want := range []string{
		"METHOD:REQUEST\r\n", "UID:bkl_1@hiringplug\r\n", "SEQUENCE:2\r\n", "DTSTART:20261110T020000Z\r\n", "DTEND:20261110T023000Z\r\n",
		"SUMMARY:Call with Aina\\; Contoh\r\n", "STATUS:CONFIRMED\r\n", "TRANSP:OPAQUE\r\n",
		"ORGANIZER;CN=\"Contoh Recruitment\":mailto:no-reply@contoh.example\r\n",
	} {
		assert.Contains(t, s, want)
	}
	assert.Contains(t, strings.ReplaceAll(s, "\r\n ", ""), `ATTENDEE;CN="Siti, A.";ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=FALSE:mailto:siti@example.com`)
	for _, line := range strings.Split(s, "\r\n") {
		assert.LessOrEqual(t, len(line), 75, line)
	}
	in.Method, in.Sequence = "CANCEL", 3
	c := in.Render(at(6, 9, 0))
	assert.Contains(t, c, "METHOD:CANCEL\r\n")
	assert.Contains(t, c, "STATUS:CANCELLED\r\n")
	assert.NotContains(t, c, "VALARM")
}
