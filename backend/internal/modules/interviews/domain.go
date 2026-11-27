package interviews

// The interview rules, as plain functions on plain values: no database, no HTTP.

import (
	"slices"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Details are the editable fields of an interview.
type Details struct {
	Title        string
	StartsAt     time.Time
	Duration     int // minutes
	Mode         string
	Location     *string
	Instructions *string
	Interviewers []string // user ids
	Status       string
}

// Patch is a partial update of Details.
type Patch struct {
	Title        apiconv.Change[string]
	StartsAt     apiconv.Change[time.Time]
	Duration     apiconv.Change[int]
	Mode         apiconv.Change[string]
	Location     apiconv.Change[*string]
	Instructions apiconv.Change[*string]
	Interviewers apiconv.Change[[]string]
	Status       apiconv.Change[string]
}

// Apply returns d with the patch's fields replaced.
func (p Patch) Apply(d Details) Details {
	apiconv.Apply(&d.Title, p.Title)
	apiconv.Apply(&d.StartsAt, p.StartsAt)
	apiconv.Apply(&d.Duration, p.Duration)
	apiconv.Apply(&d.Mode, p.Mode)
	apiconv.Apply(&d.Location, p.Location)
	apiconv.Apply(&d.Instructions, p.Instructions)
	apiconv.Apply(&d.Interviewers, p.Interviewers)
	apiconv.Apply(&d.Status, p.Status)
	return d
}

// Ends is when the interview ends.
func (d Details) Ends() time.Time { return d.StartsAt.Add(time.Duration(d.Duration) * time.Minute) }

// Normalize tidies an interview: spaces squeezed, empty text cleared, each
// interviewer once, new interviews scheduled.
func Normalize(d Details) (Details, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	d.Title = strings.Join(strings.Fields(d.Title), " ")
	if d.Title == "" {
		errs.Add("title", "is required")
	}
	if d.Status == "" {
		d.Status = "scheduled"
	}
	d.Location, d.Instructions = trimmedOrNil(d.Location), trimmedOrNil(d.Instructions)
	seen := map[string]bool{}
	people := make([]string, 0, len(d.Interviewers))
	for _, id := range d.Interviewers {
		if !seen[id] {
			seen[id] = true
			people = append(people, id)
		}
	}
	d.Interviewers = people
	if len(people) == 0 {
		errs.Add("interviewerIds", "choose at least one interviewer")
	}
	return d, errs
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// CanSchedule says why an application cannot get a new interview, or "".
func CanSchedule(stage string) string {
	switch stage {
	case "rejected", "withdrawn":
		return "This application is closed; reopen it before scheduling an interview."
	case "placed":
		return "This candidate is already placed on this job."
	}
	return ""
}

// FeedbackProblem says why a user cannot give feedback on an interview, or "".
func FeedbackProblem(userID string, d Details) string {
	if !slices.Contains(d.Interviewers, userID) {
		return "Only the interview's interviewers give feedback on it."
	}
	if d.Status == "cancelled" {
		return "The interview was cancelled."
	}
	return ""
}

// FeedbackEntry is one interviewer's feedback.
type FeedbackEntry struct {
	UserID, Recommendation string
	Comments               *string
	SubmittedAt            time.Time
}

// VisibleFeedback is the feedback a viewer may see, and how much is hidden.
// Interviewers see their colleagues' feedback once they have given their own;
// people who schedule interviews, and everyone not on the panel, see it all.
func VisibleFeedback(all []FeedbackEntry, viewer string, interviewers []string, seesAll bool) ([]FeedbackEntry, int) {
	if seesAll || !slices.Contains(interviewers, viewer) || slices.ContainsFunc(all, func(f FeedbackEntry) bool { return f.UserID == viewer }) {
		return all, 0
	}
	return []FeedbackEntry{}, len(all)
}

// FeedbackDue reports whether viewer still owes feedback on an interview that
// has taken place.
func FeedbackDue(viewer string, d Details, all []FeedbackEntry, now time.Time) bool {
	if !slices.Contains(d.Interviewers, viewer) || d.Ends().After(now) {
		return false
	}
	if d.Status != "scheduled" && d.Status != "completed" {
		return false
	}
	return !slices.ContainsFunc(all, func(f FeedbackEntry) bool { return f.UserID == viewer })
}

// OutlookChange is what an interview's organizer changed in Outlook to its
// invitation (nil: unchanged; "" clears the location or the instructions).
type OutlookChange struct {
	Title, Location, Instructions *string
	StartsAt                      *time.Time
	Duration                      *int // minutes
}

// ApplyOutlookChange takes an Outlook change into an interview, bounded to
// what the interview holds (Outlook allows longer text): an empty title keeps
// the old one, and a length HyPlug does not schedule is refused.
func ApplyOutlookChange(d Details, c OutlookChange) (Details, apperr.FieldErrors) {
	if c.Title != nil {
		if title := strings.Join(strings.Fields(*c.Title), " "); title != "" {
			d.Title = boundedRunes(title, 100)
		}
	}
	if c.Location != nil {
		location := boundedRunes(strings.TrimSpace(*c.Location), 500)
		d.Location = &location
	}
	if c.Instructions != nil {
		instructions := boundedRunes(strings.TrimSpace(*c.Instructions), 2000)
		d.Instructions = &instructions
	}
	if c.StartsAt != nil {
		d.StartsAt = *c.StartsAt
	}
	if c.Duration != nil {
		d.Duration = *c.Duration
	}
	var errs apperr.FieldErrors
	if d.Duration < 5 || d.Duration > 480 {
		errs.Add("durationMinutes", "an interview lasts 5 to 480 minutes")
	}
	d, more := Normalize(d)
	return d, append(errs, more...)
}

func boundedRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}
