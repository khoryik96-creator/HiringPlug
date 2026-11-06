package interviews

import (
	"fmt"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
)

// Calendar renders an interview as an iCalendar (.ics) file, which Outlook,
// Google Calendar and Apple Calendar import. The UID stays the same for an
// interview, and SEQUENCE follows its version, so importing it again after a
// change updates the event instead of adding a second one.
func Calendar(id string, version int, d Details, candidate, job string, now time.Time) string {
	const stamp = "20060102T150405Z"
	summary := fmt.Sprintf("%s: %s (%s)", d.Title, candidate, job)
	description := ""
	if d.Instructions != nil {
		description = *d.Instructions
	}
	location := ""
	if d.Location != nil {
		location = *d.Location
	}
	status := "CONFIRMED"
	if d.Status == "cancelled" {
		status = "CANCELLED"
	}
	lines := []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//HiringPlug//Interviews//EN", "METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + id + "@hiringplug",
		"SEQUENCE:" + fmt.Sprint(version),
		"DTSTAMP:" + now.UTC().Format(stamp),
		"DTSTART:" + d.StartsAt.UTC().Format(stamp),
		"DTEND:" + d.Ends().UTC().Format(stamp),
		"SUMMARY:" + ical.Escape(summary),
		"STATUS:" + status,
	}
	if location != "" {
		lines = append(lines, "LOCATION:"+ical.Escape(location))
	}
	if description != "" {
		lines = append(lines, "DESCRIPTION:"+ical.Escape(description))
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(ical.Fold(l))
		b.WriteString("\r\n")
	}
	return b.String()
}
