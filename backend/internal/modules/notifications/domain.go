package notifications

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// The kinds of notification (NotificationKind in the contract).
const (
	KindApplicant     = "applicant"
	KindTask          = "task"
	KindOfferApproval = "offer_approval"
	KindInterview     = "interview"
)

// Kinds is every kind, in the order Settings lists them.
var Kinds = []string{KindApplicant, KindTask, KindOfferApproval, KindInterview}

// NormalizeMuted checks the kinds a user turned off and puts them in order,
// once each.
func NormalizeMuted(kinds []string) ([]string, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	for _, k := range kinds {
		if !slices.Contains(Kinds, k) {
			errs.Add("mutedKinds", "is not a kind of notification: "+k)
		}
	}
	return NormalizeMutedSilently(kinds), errs
}

// NormalizeMutedSilently is NormalizeMuted without the check (for what is stored).
func NormalizeMutedSilently(kinds []string) []string {
	out := []string{}
	for _, k := range Kinds {
		if slices.Contains(kinds, k) {
			out = append(out, k)
		}
	}
	return out
}

// ApplicantDays is how far back new applicants show in the bell.
const ApplicantDays = 7

// MorningHour is when the morning email goes, in the agency's time.
const MorningHour = 8

// MaxItems is the most the bell shows.
const MaxItems = 100

// Record is an application or offer and when it happened.
type Record struct {
	ID, ApplicationID string
	At                time.Time
}

// Task is an open task of the user's that is due.
type Task struct {
	ID, Title     string
	DueOn         time.Time // a day: its year, month and day count, not its zone
	ApplicationID *string
	CreatedAt     time.Time
}

// Interview is a scheduled interview of the user's.
type Interview struct {
	ID, ApplicationID, Title string
	StartsAt, CreatedAt      time.Time
}

// Application is who and which job an application is.
type Application struct {
	CandidateID, CandidateName string
	JobTitle, CompanyName      string
}

// Inputs are the records a user's notifications are made from.
type Inputs struct {
	Applicants, Offers []Record
	Tasks              []Task
	Interviews         []Interview
	Apps               map[string]Application
}

// Item is one notification.
type Item struct {
	ID, Kind, Title, Detail, Href string
	At                            time.Time // when it became a notification
	New                           bool
}

// Window is what the notifications cover at a moment: tasks due up to
// today, interviews from now to the end of tomorrow, applicants since a
// week ago (all in the agency's time).
type Window struct {
	Today           time.Time // midnight UTC of the agency's today (compared with due days)
	From, To        time.Time // interviews starting in [From, To)
	ApplicantsSince time.Time
	zone            *time.Location
}

// WindowAt returns the window at now in loc.
func WindowAt(now time.Time, loc *time.Location) Window {
	local := now.In(loc)
	y, m, d := local.Date()
	return Window{
		Today:           time.Date(y, m, d, 0, 0, 0, 0, time.UTC),
		From:            now,
		To:              time.Date(y, m, d+2, 0, 0, 0, 0, loc),
		ApplicantsSince: now.AddDate(0, 0, -ApplicantDays),
		zone:            loc,
	}
}

// Build makes the notifications, newest first (at most MaxItems), marking
// as new those after seen (all of them when the user never looked), and
// returns how many are new.
func Build(in Inputs, seen *time.Time, w Window) ([]Item, int) {
	loc := w.zone
	if loc == nil {
		loc = time.UTC
	}
	var out []Item
	about := func(appID string) (Application, bool) { a, ok := in.Apps[appID]; return a, ok }
	for _, r := range in.Applicants {
		a, ok := about(r.ApplicationID)
		if !ok {
			continue
		}
		out = append(out, Item{ID: KindApplicant + ":" + r.ApplicationID, Kind: KindApplicant, At: r.At,
			Title: a.CandidateName + " applied", Detail: jobText(a), Href: "#/candidates/" + a.CandidateID})
	}
	for _, t := range in.Tasks {
		due := dayIn(t.DueOn, loc)
		it := Item{ID: KindTask + ":" + t.ID, Kind: KindTask, Title: t.Title, At: later(t.CreatedAt, due), Href: "#/"}
		if due.Before(dayIn(w.Today, loc)) {
			it.Detail = "Overdue, due " + due.Format("Mon 2 Jan")
		} else {
			it.Detail = "Due today"
		}
		if t.ApplicationID != nil {
			if a, ok := about(*t.ApplicationID); ok {
				it.Detail += " · " + a.CandidateName + ", " + a.JobTitle
				it.Href = "#/candidates/" + a.CandidateID
			}
		}
		out = append(out, it)
	}
	for _, r := range in.Offers {
		a, ok := about(r.ApplicationID)
		if !ok {
			continue
		}
		out = append(out, Item{ID: KindOfferApproval + ":" + r.ID, Kind: KindOfferApproval, At: r.At,
			Title: "Offer for " + a.CandidateName + " needs your approval", Detail: jobText(a), Href: "#/"})
	}
	for _, iv := range in.Interviews {
		a, ok := about(iv.ApplicationID)
		if !ok {
			continue
		}
		starts := iv.StartsAt.In(loc)
		y, m, d := starts.Date()
		dayBefore := time.Date(y, m, d-1, 0, 0, 0, 0, loc)
		out = append(out, Item{ID: KindInterview + ":" + iv.ID, Kind: KindInterview, At: later(iv.CreatedAt, dayBefore),
			Title:  "Interview " + dayWord(starts, w.Today) + ", " + starts.Format("3:04 PM") + ": " + a.CandidateName,
			Detail: iv.Title + " · " + jobText(a), Href: "#/candidates/" + a.CandidateID})
	}
	slices.SortStableFunc(out, func(a, b Item) int { return cmp.Or(b.At.Compare(a.At), strings.Compare(a.ID, b.ID)) })
	if len(out) > MaxItems {
		out = out[:MaxItems]
	}
	unseen := 0
	for i := range out {
		out[i].New = seen == nil || out[i].At.After(*seen)
		if out[i].New {
			unseen++
		}
	}
	return out, unseen
}

func jobText(a Application) string {
	if a.CompanyName == "" {
		return a.JobTitle
	}
	return a.JobTitle + ", " + a.CompanyName
}

// dayIn is midnight of a date's day in loc.
func dayIn(day time.Time, loc *time.Location) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// dayWord is "today", "tomorrow" or the weekday of a local time, against
// today (midnight UTC of the agency's day).
func dayWord(local, today time.Time) string {
	y, m, d := local.Date()
	switch time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Sub(today) {
	case 0:
		return "today"
	case 24 * time.Hour:
		return "tomorrow"
	}
	return local.Format("Mon 2 Jan")
}

// NextMorning is the next MorningHour o'clock in loc strictly after now.
func NextMorning(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	y, m, d := local.Date()
	at := time.Date(y, m, d, MorningHour, 0, 0, 0, loc)
	if !at.After(now) {
		at = time.Date(y, m, d+1, MorningHour, 0, 0, 0, loc)
	}
	return at
}

// DigestFor picks what the morning email lists: everything in the bell
// except applicants older than a day (yesterday's email had them).
func DigestFor(items []Item, now time.Time) []Item {
	var out []Item
	for _, it := range items {
		if it.Kind == KindApplicant && it.At.Before(now.Add(-24*time.Hour)) {
			continue
		}
		out = append(out, it)
	}
	return out
}

var digestSections = []struct{ kind, heading string }{
	{KindInterview, "Interviews today and tomorrow"},
	{KindTask, "Tasks due"},
	{KindOfferApproval, "Offers waiting for your approval"},
	{KindApplicant, "New applicants on your jobs"},
}

// DigestText writes the morning email; false when there is nothing to say.
func DigestText(items []Item, publicURL string) (subject, body string, ok bool) {
	if len(items) == 0 {
		return "", "", false
	}
	count := map[string]int{}
	for _, it := range items {
		count[it.Kind]++
	}
	var parts []string
	var b strings.Builder
	b.WriteString("Good morning. Here is what needs you today.\n")
	for _, sec := range digestSections {
		n := count[sec.kind]
		if n == 0 {
			continue
		}
		parts = append(parts, countText(sec.kind, n))
		b.WriteString("\n" + sec.heading + "\n")
		for _, it := range items {
			if it.Kind == sec.kind {
				fmt.Fprintf(&b, "- %s (%s)\n  %s/%s\n", it.Title, it.Detail, strings.TrimRight(publicURL, "/"), it.Href)
			}
		}
	}
	b.WriteString("\nTo stop these emails, open the bell in HiringPlug and untick \"Email me this each morning\".\n\nHiringPlug")
	return "Today: " + strings.Join(parts, ", "), b.String(), true
}

func countText(kind string, n int) string {
	unit := map[string][2]string{
		KindInterview:     {"interview", "interviews"},
		KindTask:          {"task due", "tasks due"},
		KindOfferApproval: {"offer to approve", "offers to approve"},
		KindApplicant:     {"new applicant", "new applicants"},
	}[kind]
	if n == 1 {
		return "1 " + unit[0]
	}
	return fmt.Sprintf("%d %s", n, unit[1])
}
