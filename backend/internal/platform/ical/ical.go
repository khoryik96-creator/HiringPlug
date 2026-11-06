// Package ical writes calendar invitations and reads busy times from
// calendar addresses (RFC 5545), for Google Calendar, Outlook and Apple
// Calendar alike.
package ical

import (
	"fmt"
	"strings"
	"time"
)

// Person is an organizer or attendee.
type Person struct{ Name, Email string }

// Invite is one event sent as an invitation (RFC 5546): METHOD:REQUEST to
// add or change it, CANCEL to remove it. The UID stays the same for a
// meeting and Sequence goes up with each change, so calendars update the
// event instead of adding another.
type Invite struct {
	Method    string // REQUEST or CANCEL
	UID       string
	Sequence  int
	Organizer Person
	Attendees []Person
	Start     time.Time
	End       time.Time
	Summary   string
	Location  string
	Body      string
}

const stamp = "20060102T150405Z"

// Render writes the invitation, lines folded at 75 bytes and ending in CRLF.
func (in Invite) Render(now time.Time) string {
	status := "CONFIRMED"
	if in.Method == "CANCEL" {
		status = "CANCELLED"
	}
	lines := []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//HiringPlug//Scheduling//EN", "CALSCALE:GREGORIAN", "METHOD:" + in.Method,
		"BEGIN:VEVENT",
		"UID:" + in.UID,
		"SEQUENCE:" + fmt.Sprint(in.Sequence),
		"DTSTAMP:" + now.UTC().Format(stamp),
		"DTSTART:" + in.Start.UTC().Format(stamp),
		"DTEND:" + in.End.UTC().Format(stamp),
		"SUMMARY:" + Escape(in.Summary),
		"STATUS:" + status,
		"TRANSP:OPAQUE", // busy in every calendar it reaches
		"ORGANIZER;CN=" + param(in.Organizer.Name) + ":mailto:" + in.Organizer.Email,
	}
	for _, a := range in.Attendees {
		lines = append(lines, "ATTENDEE;CN="+param(a.Name)+";ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=FALSE:mailto:"+a.Email)
	}
	if in.Location != "" {
		lines = append(lines, "LOCATION:"+Escape(in.Location))
	}
	if in.Body != "" {
		lines = append(lines, "DESCRIPTION:"+Escape(in.Body))
	}
	if in.Method == "REQUEST" {
		lines = append(lines, "BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:"+Escape(in.Summary), "TRIGGER:-PT15M", "END:VALARM")
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(Fold(l))
		b.WriteString("\r\n")
	}
	return b.String()
}

// param quotes a parameter value (names may have commas or colons).
func param(s string) string {
	return `"` + strings.NewReplacer(`"`, "'", "\r", "", "\n", " ").Replace(s) + `"`
}

// Escape follows RFC 5545: backslashes, commas, semicolons and line breaks.
func Escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, ",", `\,`, ";", `\;`, "\r\n", `\n`, "\n", `\n`, "\r", "").Replace(s)
}

// Fold breaks lines longer than 75 bytes, as the format requires, without
// splitting a character.
func Fold(line string) string {
	var b strings.Builder
	width := 0
	for _, r := range line {
		n := len(string(r))
		if width+n > 75 {
			b.WriteString("\r\n ")
			width = 1
		}
		b.WriteRune(r)
		width += n
	}
	return b.String()
}
