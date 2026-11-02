package emails

// Placeholder values, as plain functions on plain values: no database.

import (
	"fmt"
	"strings"
	"time"
)

// Context is what a draft is about: the candidate, and perhaps an
// application and an interview.
type Context struct {
	Candidate   Candidate
	Application *Application
	Interview   *Interview
	SenderName  string
	AgencyName  string
	Location    *time.Location // the sender's time zone, for interview times
}

// Values returns the placeholder values for a draft. A placeholder whose
// record is not part of the draft has no value (and is reported missing).
func Values(c Context) map[string]string {
	v := map[string]string{
		"candidate.firstName": c.Candidate.FirstName,
		"candidate.fullName":  c.Candidate.FullName,
		"sender.name":         c.SenderName,
		"agency.name":         c.AgencyName,
	}
	if v["candidate.firstName"] == "" {
		v["candidate.firstName"] = c.Candidate.FullName
	}
	if a := c.Application; a != nil {
		v["job.title"] = a.JobTitle
		if a.CompanyName != "" {
			v["company.name"] = a.CompanyName
		}
	}
	if iv := c.Interview; iv != nil {
		v["interview.title"] = iv.Title
		v["interview.when"] = When(iv.StartsAt, c.Location)
		v["interview.length"] = Length(iv.Minutes)
		v["interview.how"] = modeLabel[iv.Mode]
		if iv.Location != nil && *iv.Location != "" {
			v["interview.where"] = *iv.Location
		}
		v["interview.instructions"] = ""
		if iv.Instructions != nil {
			v["interview.instructions"] = *iv.Instructions
		}
	}
	return v
}

var modeLabel = map[string]string{"in_person": "In person", "video": "Video call", "phone": "Phone call"}

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
