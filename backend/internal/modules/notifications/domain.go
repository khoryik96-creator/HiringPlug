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
	KindCVFormatted   = "cv_formatted"
	KindClientAnswer  = "client_answer"
	KindAILimit       = "ai_limit"
)

// Kinds is every kind, in the order Settings lists them.
var Kinds = []string{KindApplicant, KindTask, KindOfferApproval, KindInterview, KindCVFormatted, KindClientAnswer, KindAILimit}

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

// FormattedDays is how far back CVs formatted show in the bell.
const FormattedDays = 2

// AnswerDays is how far back a client's answers on shortlist links show in the bell.
const AnswerDays = 7

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

// FormattedCV is a CV the user had formatted CV Studio's way: ready, or failed.
type FormattedCV struct {
	ID, CandidateID, CandidateName string
	Blind, Failed                  bool
	At                             time.Time // when it finished
}

// ClientAnswer is a client's answer on a shortlist link the user sent.
type ClientAnswer struct {
	ID, ApplicationID, JobID string
	Approved                 bool
	Comment                  string
	At                       time.Time
}

// AILimitLevel is the level of the whole AI limit (an AILimit's Level
// otherwise is the company's warning percent).
const AILimitLevel = 100

// AILimit is a level the company's AI use reached in a month.
type AILimit struct {
	Level int       // the warning percent, or AILimitLevel
	At    time.Time // when the use reached it
	Month time.Time // when the month began
	Until time.Time // when the next month begins (the limit starts again)
}

// Inputs are the records a user's notifications are made from.
type Inputs struct {
	Applicants, Offers []Record
	Tasks              []Task
	Interviews         []Interview
	FormattedCVs       []FormattedCV
	ClientAnswers      []ClientAnswer
	AILimits           []AILimit
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
// week ago, CVs formatted in the last two days (all in the agency's time).
type Window struct {
	Today           time.Time // midnight UTC of the agency's today (compared with due days)
	From, To        time.Time // interviews starting in [From, To)
	ApplicantsSince time.Time
	FormattedSince  time.Time
	AnswersSince    time.Time
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
		FormattedSince:  now.AddDate(0, 0, -FormattedDays),
		AnswersSince:    now.AddDate(0, 0, -AnswerDays),
		zone:            loc,
	}
}

// Build makes the notifications, newest first, marking as new those after
// seen (all of them when the user never looked).
func Build(in Inputs, seen *time.Time, w Window) []Item {
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
	for _, f := range in.FormattedCVs {
		it := Item{ID: KindCVFormatted + ":" + f.ID, Kind: KindCVFormatted, At: f.At, Href: "#/candidates/" + f.CandidateID}
		what := "CV"
		if f.Blind {
			what = "Blind CV"
		}
		if f.Failed {
			it.Title, it.Detail = what+" could not be formatted: "+f.CandidateName, "Try again from their page"
		} else {
			it.Title, it.Detail = what+" formatted: "+f.CandidateName, "Ready under CVs on their page"
		}
		out = append(out, it)
	}
	for _, c := range in.ClientAnswers {
		a, ok := about(c.ApplicationID)
		if !ok {
			continue
		}
		it := Item{ID: KindClientAnswer + ":" + c.ID, Kind: KindClientAnswer, At: c.At, Detail: jobText(a), Href: "#/jobs/" + c.JobID}
		if c.Approved {
			it.Title = "Client approved " + a.CandidateName
		} else {
			it.Title = "Client rejected " + a.CandidateName
		}
		if comment := strings.Join(strings.Fields(c.Comment), " "); comment != "" {
			it.Detail += ` · "` + clip(comment, 140) + `"`
		}
		out = append(out, it)
	}
	out = append(out, aiLimitItems(in.AILimits, loc)...)
	slices.SortStableFunc(out, func(a, b Item) int { return cmp.Or(b.At.Compare(a.At), strings.Compare(a.ID, b.ID)) })
	for i := range out {
		out[i].New = seen == nil || out[i].At.After(*seen)
	}
	return out
}

// Shown is what the bell shows: the newest MaxItems, and how many of them
// are new. (The morning email picks from all of them: DigestFor.)
func Shown(items []Item) ([]Item, int) {
	if len(items) > MaxItems {
		items = items[:MaxItems]
	}
	unseen := 0
	for _, it := range items {
		if it.New {
			unseen++
		}
	}
	return items, unseen
}

// aiLimitItems is one item per month: the whole limit once it is reached,
// the warning before that.
func aiLimitItems(levels []AILimit, loc *time.Location) []Item {
	top := map[string]AILimit{}
	var months []string
	for _, l := range levels {
		month := l.Month.In(loc).Format("2006-01")
		have, ok := top[month]
		if !ok {
			months = append(months, month)
		}
		if !ok || l.Level > have.Level {
			top[month] = l
		}
	}
	out := make([]Item, 0, len(months))
	for _, month := range months {
		l := top[month]
		until := l.Until.In(loc).Format("2 Jan")
		it := Item{ID: fmt.Sprintf("%s:%s:%d", KindAILimit, month, l.Level), Kind: KindAILimit, At: l.At, Href: "#/settings/ai-usage"}
		if l.Level >= AILimitLevel {
			it.Title = "AI limit reached: AI features are paused until " + until
			it.Detail = "See what used it in Settings › AI usage"
		} else {
			it.Title = fmt.Sprintf("AI use reached %d%% of this month's limit", l.Level)
			it.Detail = "AI features pause at 100%, until " + until
		}
		out = append(out, it)
	}
	return out
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
	day := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if day.Equal(today) {
		return "today"
	}
	if day.Equal(today.AddDate(0, 0, 1)) {
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

// DigestFor picks what the morning email lists: everything, except
// applicants and AI limit notices from before since (the last email's
// morning: that email had them).
func DigestFor(items []Item, since time.Time) []Item {
	var out []Item
	for _, it := range items {
		if it.Kind == KindCVFormatted { // for the moment it is ready, in the bell; not the morning email
			continue
		}
		if (it.Kind == KindApplicant || it.Kind == KindClientAnswer || it.Kind == KindAILimit) && it.At.Before(since) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// DigestSince is when the applicants in a morning email start: the morning
// of the last email (emailedOn, a day; zero when there was none), or a day
// before now for the first.
func DigestSince(emailedOn time.Time, now time.Time, loc *time.Location) time.Time {
	if emailedOn.IsZero() {
		return now.Add(-24 * time.Hour)
	}
	y, m, d := emailedOn.Date()
	return time.Date(y, m, d, MorningHour, 0, 0, 0, loc)
}

// NextDayMorning is MorningHour o'clock on the agency's day after now's: a
// morning's job queues the next from its own day, so a worker whose clock
// is a little behind the database's never queues the same morning again.
func NextDayMorning(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d+1, MorningHour, 0, 0, 0, loc)
}

var digestSections = []struct{ kind, heading string }{
	{KindInterview, "Interviews today and tomorrow"},
	{KindTask, "Tasks due"},
	{KindOfferApproval, "Offers waiting for your approval"},
	{KindApplicant, "New applicants on your jobs"},
	{KindClientAnswer, "Clients' answers on your shortlist links"},
	{KindAILimit, "The company's AI use this month"},
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
	b.WriteString("\nTo choose what you get, or stop these emails: Settings > Your account > Notifications.\n\nHyPlug")
	return "Today: " + strings.Join(parts, ", "), b.String(), true
}

func countText(kind string, n int) string {
	unit := map[string][2]string{
		KindInterview:     {"interview", "interviews"},
		KindTask:          {"task due", "tasks due"},
		KindOfferApproval: {"offer to approve", "offers to approve"},
		KindApplicant:     {"new applicant", "new applicants"},
		KindClientAnswer:  {"client answer", "client answers"},
		KindAILimit:       {"AI limit notice", "AI limit notices"},
	}[kind]
	if n == 1 {
		return "1 " + unit[0]
	}
	return fmt.Sprintf("%d %s", n, unit[1])
}

// clip shortens text to at most n characters, ending in "…" when cut.
func clip(text string, n int) string {
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
