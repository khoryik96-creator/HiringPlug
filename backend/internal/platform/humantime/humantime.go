// Package humantime writes times and durations the way people read them in
// emails and messages.
package humantime

import (
	"fmt"
	"strings"
	"time"
)

// When writes a time for a reader: "Tuesday, 10 November 2026, 10:00 AM (GMT+8)".
func When(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	t = t.In(loc)
	_, offset := t.Zone()
	zone := "GMT"
	if offset != 0 {
		sign := "+"
		if offset < 0 {
			sign, offset = "-", -offset
		}
		zone += sign + fmt.Sprint(offset/3600)
		if m := offset % 3600 / 60; m != 0 {
			zone += fmt.Sprintf(":%02d", m)
		}
	}
	return t.Format("Monday, 2 January 2006, 3:04 PM") + " (" + zone + ")"
}

// Length writes a duration in minutes: "45 minutes", "1 hour", "1 hour 30 minutes".
func Length(minutes int) string {
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	h, m := minutes/60, minutes%60
	var parts []string
	if h > 0 {
		parts = append(parts, plural(h, "hour"))
	}
	if m > 0 || h == 0 {
		parts = append(parts, plural(m, "minute"))
	}
	return strings.Join(parts, " ")
}
