package auditlog

// The rules, as pure functions on plain values.

import (
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Actor kinds.
const (
	ActorUser   = "user"
	ActorAPIKey = "api_key"
	ActorSystem = "system"
)

// systemID is the actor of what happened automatically (authz.SystemUserID).
const systemID = "system"

// Names shown for actors that are not a person.
const (
	AutomaticName = "Automatic"
	APIKeyName    = "API key"
)

// ActorKind is who an actor id stands for: the system, an API key or a person.
func ActorKind(id string) string {
	switch {
	case id == systemID || id == "":
		return ActorSystem
	case strings.HasPrefix(id, "akey_"):
		return ActorAPIKey
	default:
		return ActorUser
	}
}

// ActorName is how an actor is shown: "Automatic" for the system; an API
// key's name, or "API key" when it is not found; a person's name, or their
// id when it is not found.
func ActorName(id string, users, keys map[string]string) string {
	switch ActorKind(id) {
	case ActorSystem:
		return AutomaticName
	case ActorAPIKey:
		if name := keys[id]; name != "" {
			return name
		}
		return APIKeyName
	default:
		if name := users[id]; name != "" {
			return name
		}
		return id
	}
}

// SplitActions reads the action filter: comma-separated, each with a dot
// one action ("candidate.updated"), without every action of that kind
// ("candidate"). Blanks and repeats are left out.
func SplitActions(filter *string) (actions, kinds []string) {
	if filter == nil {
		return nil, nil
	}
	for _, a := range Distinct(strings.Split(*filter, ",")) {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		if strings.Contains(a, ".") {
			actions = append(actions, a)
		} else {
			kinds = append(kinds, a)
		}
	}
	return actions, kinds
}

// Day is the day a moment falls on in loc, as midnight UTC of that date
// (how a date column is written).
func Day(at time.Time, loc *time.Location) time.Time {
	y, m, d := at.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DayRange turns the days a filter names (in loc; either may be missing)
// into moments: from the start of from up to, not including, the start of
// the day after to.
func DayRange(from, to *time.Time, loc *time.Location) (start, end *time.Time) {
	if from != nil {
		s := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
		start = &s
	}
	if to != nil {
		e := time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, loc)
		end = &e
	}
	return start, end
}

// A cursor says where a page ended: its last line's time (an entry) or day
// (a salary view), "~", and its id. The next page follows on from there,
// even when that line is gone (a merge folds a duplicate's line into the
// kept candidate's).
const cursorSep = "~"

// LogCursor is the cursor after an audit log entry.
func LogCursor(at time.Time, id string) string {
	return at.UTC().Format(time.RFC3339Nano) + cursorSep + id
}

// ViewCursor is the cursor after a salary view line.
func ViewCursor(day time.Time, id string) string {
	return day.Format(time.DateOnly) + cursorSep + id
}

// ReadLogCursor reads LogCursor; ok is false for anything else.
func ReadLogCursor(c string) (at time.Time, id string, ok bool) {
	when, id, found := strings.Cut(c, cursorSep)
	if !found || !strings.HasPrefix(id, "aud_") {
		return time.Time{}, "", false
	}
	at, err := time.Parse(time.RFC3339Nano, when)
	return at, id, err == nil
}

// ReadViewCursor reads ViewCursor; ok is false for anything else.
func ReadViewCursor(c string) (day time.Time, id string, ok bool) {
	when, id, found := strings.Cut(c, cursorSep)
	if !found || !strings.HasPrefix(id, "sv_") {
		return time.Time{}, "", false
	}
	day, err := time.Parse(time.DateOnly, when)
	return day, id, err == nil
}

// IsKind reports whether kind is a salary view kind (salaryview.Kinds).
func IsKind(kind string) bool { _, ok := KindLabels[kind]; return ok }

// Distinct is ids without blanks or repeats, in their first order.
func Distinct(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// InLockOrder is ids without blanks or repeats, sorted: the order a
// showing counts its candidates' lines in, the same in every transaction, so
// two counted at once never wait on each other in a circle.
func InLockOrder(ids []string) []string {
	out := Distinct(ids)
	sort.Strings(out)
	return out
}

// KindLabels are the words for each salary view kind (salaryview.Kinds).
var KindLabels = map[string]string{
	"candidate": "Candidate page", "candidate_list": "Candidate list or search", "export": "Export",
	"email_fields": "Email being written", "email_sent": "Email sent", "email_read": "Email read", "cv": "Formatted CV",
	"screening": "Screening call", "offer": "Offer", "placement": "Placement",
}

// Grouped kinds are one line a day for many candidates (salaryview.Grouped).
func Grouped(kind string) bool { return kind == "candidate_list" || kind == "export" }

// RecordKinds are the entity types whose names an entry shows through
// Deps.Records (applications are named through Deps.Applications; the
// others have none); jobs are written as both job and job_order.
var RecordKinds = []string{"candidate", "job", "job_order", "company", "contact", "job_lead"}

// ApplicationName is how an entry names a candidate on a job: "Siti Aisyah
// · Accountant" (either alone when the other is missing).
func ApplicationName(candidate, job string) string {
	switch {
	case candidate == "":
		return job
	case job == "":
		return candidate
	}
	return candidate + " · " + job
}

// CellLimit is the most characters an Excel cell holds.
const CellLimit = 32767

// FitCell cuts text that would not fit in a spreadsheet cell, saying how
// much was left out (the whole entry is in the app).
func FitCell(text string) string {
	n := utf8.RuneCountInString(text)
	if n <= CellLimit {
		return text
	}
	runes := []rune(text)
	for keep := CellLimit - 60; keep > 0; keep -= 10 {
		note := "… (" + strconv.Itoa(n-keep) + " more characters: see the entry in the app)"
		if keep+utf8.RuneCountInString(note) <= CellLimit {
			return string(runes[:keep]) + note
		}
	}
	return string(runes[:CellLimit])
}
