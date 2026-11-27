package notifications

// The morning email's monthly line about stale records: on the first
// morning of each month, how many contacts and companies nobody touched for
// a year (not edited, no note, no email, no job), with links to those lists.

import (
	"fmt"
	"strings"
	"time"
)

// Stale is how many contacts and companies nobody touched for a year, of
// those the user may see.
type Stale struct{ Contacts, Companies int }

func (s Stale) empty() bool { return s.Contacts == 0 && s.Companies == 0 }

// StaleDay reports whether now is the first day of a month in loc: the
// morning the email says how many records went stale.
func StaleDay(now time.Time, loc *time.Location) bool { return now.In(loc).Day() == 1 }

// StaleSinceAt is the time before which a record nobody touched counts as stale: a year before now.
func StaleSinceAt(now time.Time) time.Time { return now.AddDate(-1, 0, 0) }

// DigestWith writes the morning email with the monthly stale-records part
// (none when stale is empty); false when there is nothing to say.
func DigestWith(items []Item, stale Stale, publicURL string) (subject, body string, ok bool) {
	subject, body, ok = DigestText(items, publicURL)
	if stale.empty() {
		return subject, body, ok
	}
	base := strings.TrimRight(publicURL, "/")
	var lines []string
	if stale.Contacts > 0 {
		lines = append(lines, fmt.Sprintf("- %s\n  %s/#/contacts?stale=1", plural(stale.Contacts, "contact", "contacts"), base))
	}
	if stale.Companies > 0 {
		lines = append(lines, fmt.Sprintf("- %s\n  %s/#/companies?stale=1", plural(stale.Companies, "company", "companies"), base))
	}
	part := "\nNot touched for a year (no edit, note, email or job): worth a check this month\n" + strings.Join(lines, "\n") + "\n"
	note := plural(stale.Contacts+stale.Companies, "record", "records") + " not touched for a year"
	if !ok {
		return "This month: " + note, "Good morning." + "\n" + part +
			"\nTo choose what you get, or stop these emails: Settings > Your account > Notifications.\n\nHiringPlug", true
	}
	const foot = "\nTo choose what you get"
	at := strings.Index(body, foot)
	return subject + ", " + note, body[:at] + part + body[at:], true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
