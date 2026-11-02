package emails

// Placeholder values, as plain functions on plain values: no database.

import (
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/humantime"
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
		v["interview.when"] = humantime.When(iv.StartsAt, c.Location)
		v["interview.length"] = humantime.Length(iv.Minutes)
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
