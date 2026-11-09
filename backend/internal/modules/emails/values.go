package emails

// Placeholder values, as plain functions on plain values: no database.

import (
	"strconv"
	"strings"
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
	if c.Candidate.LastName != "" {
		v["candidate.lastName"] = c.Candidate.LastName
	}
	if d := c.Candidate.Details; d != nil {
		for key, val := range map[string]*string{
			"candidate.nationality": d.Nationality, "candidate.residence": d.Residence,
			"candidate.currentTitle": d.CurrentTitle, "candidate.currentCompany": d.CurrentCompany,
			"candidate.noticePeriod": d.NoticePeriod, "candidate.workAuthorization": d.WorkAuthorization,
			"candidate.currentSalary": d.CurrentSalary, "candidate.expectedSalary": d.ExpectedSalary,
		} {
			if val != nil && strings.TrimSpace(*val) != "" {
				v[key] = *val
			}
		}
	}
	if a := c.Application; a != nil {
		v["job.title"] = a.JobTitle
		if a.CompanyName != "" {
			v["company.name"] = a.CompanyName
		}
		if a.ClientContact != "" {
			v["client.name"] = a.ClientContact
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

// FormatSalary writes a salary as an email shows it: "RM 9,500 per month".
func FormatSalary(amount, currency, period string) string {
	return money(currency, amount) + " per " + period
}

// FormatRange writes an expected salary: "RM 12,000 per month", or
// "RM 12,000 – 14,000 per month".
func FormatRange(minimum string, maximum *string, currency, period string) string {
	if maximum == nil || *maximum == "" || *maximum == minimum {
		return FormatSalary(minimum, currency, period)
	}
	return money(currency, minimum) + " – " + grouped(*maximum) + " per " + period
}

func money(currency, amount string) string {
	symbol := currency
	if currency == "MYR" {
		symbol = "RM"
	}
	return symbol + " " + grouped(amount)
}

// grouped writes "9500.00" as "9,500" and "9500.50" as "9,500.50".
func grouped(amount string) string {
	whole, cents, _ := strings.Cut(amount, ".")
	if strings.Trim(cents, "0") == "" {
		cents = ""
	}
	n := len(whole)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if cents != "" {
		return b.String() + "." + cents
	}
	return b.String()
}

// FormatNotice writes a notice period: "Immediate", "1 month", "2 weeks".
func FormatNotice(value int, unit string) string {
	if value == 0 {
		return "Immediate"
	}
	if value == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(value) + " " + unit + "s"
}

// FormatWorkAuthorization writes the expat status: "Local", "Permanent
// resident", "Expat (RP-T)", "Expat (spouse visa)", "Expat (other pass)",
// "Needs an employment pass".
func FormatWorkAuthorization(status string, basis *string) string {
	switch status {
	case "local":
		return "Local"
	case "requires_visa":
		return "Needs an employment pass"
	}
	switch {
	case basis == nil:
		return "Expat, can work here"
	case *basis == "permanent_resident":
		return "Permanent resident"
	case *basis == "rp_t":
		return "Expat (RP-T)"
	case *basis == "spouse_visa":
		return "Expat (spouse visa)"
	default:
		return "Expat (other pass)"
	}
}

// ContactValues are the field values for an email to one of a client's
// contacts: [First name], [Last name] and [Full name] are the contact's,
// [Client company] their company, [Client contact] them too.
func ContactValues(ct Contact, senderName, agencyName string) map[string]string {
	v := map[string]string{"sender.name": senderName, "agency.name": agencyName}
	for key, val := range map[string]string{
		"candidate.firstName": ct.FirstName, "candidate.lastName": ct.LastName, "candidate.fullName": ct.FullName,
		"client.name": ct.FullName, "company.name": ct.CompanyName,
	} {
		if strings.TrimSpace(val) != "" {
			v[key] = val
		}
	}
	if v["candidate.firstName"] == "" && ct.FullName != "" {
		v["candidate.firstName"] = ct.FullName
	}
	return v
}
