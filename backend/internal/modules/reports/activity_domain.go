package reports

// Activity against targets, as plain functions on plain values: which week or
// month a day is in, what each user did in it, their target, and the checks
// on the targets and weights a person saves.

import (
	"strconv"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Periods of activity targets.
const (
	Week  = "week"
	Month = "month"
)

// PeriodOf returns the first and last day of the week (Monday to Sunday) or
// month that day is in.
func PeriodOf(period string, day time.Time) (time.Time, time.Time) {
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	if period == Month {
		first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
		return first, first.AddDate(0, 1, -1)
	}
	back := (int(day.Weekday()) + 6) % 7 // days since Monday
	first := day.AddDate(0, 0, -back)
	return first, first.AddDate(0, 0, 6)
}

// Counts are what a user did: calls logged, CVs sent to clients, interviews
// arranged, placements recorded, and candidates moved into a stage of each
// category (by category key).
type Counts struct {
	Calls, CVsSent, Interviews, Placements int
	Categories                             map[string]int
}

// Goal is a target for each activity; nil when none is set for it. A
// category without a target is left out of Categories.
type Goal struct {
	Calls, CVsSent, Interviews, Placements *int
	Categories                             map[string]int
}

// Facts are what users did in a period, as read from other modules.
type Facts struct {
	ScreeningCalls []ScreeningCall
	CallNotes      []CallNote
	ScreeningNotes map[string]bool // notes written with a screening call: the call itself
	Submitted      []CVSent
	Emailed        []CVSent
	Interviews     []Done
	Placements     []Done
	CategoryMoves  []CategoryMove
}

// CategoryMove is a user moving an application into a stage of a category.
type CategoryMove struct {
	By, ApplicationID, Category string
	At                          time.Time
}

// CountActivity adds up each user's activity. A screening call counts once
// (its note is the call); a CV sent counts once per candidate per day for the
// same user, however it was sent (moved to submitted and emailed). day gives
// the company's day of an instant.
func CountActivity(f Facts, day func(time.Time) time.Time) map[string]*Counts {
	out := map[string]*Counts{}
	of := func(user string) *Counts {
		if out[user] == nil {
			out[user] = &Counts{}
		}
		return out[user]
	}
	for _, c := range f.ScreeningCalls {
		of(c.By).Calls++
	}
	for _, n := range f.CallNotes {
		if n.Kind == "screening" && f.ScreeningNotes[n.ID] {
			continue
		}
		of(n.By).Calls++
	}
	seen := map[string]bool{}
	for _, cv := range append(append([]CVSent{}, f.Submitted...), f.Emailed...) {
		key := cv.By + "|" + cv.CandidateID + "|" + day(cv.At).Format(time.DateOnly)
		if seen[key] {
			continue
		}
		seen[key] = true
		of(cv.By).CVsSent++
	}
	for _, d := range f.Interviews {
		of(d.By).Interviews++
	}
	for _, d := range f.Placements {
		of(d.By).Placements++
	}
	// A candidate counts once per category however often they are moved back
	// into it.
	moved := map[string]bool{}
	for _, m := range f.CategoryMoves {
		key := m.By + "|" + m.ApplicationID + "|" + m.Category
		if moved[key] {
			continue
		}
		moved[key] = true
		c := of(m.By)
		if c.Categories == nil {
			c.Categories = map[string]int{}
		}
		c.Categories[m.Category]++
	}
	// The app's own work (a worker, an accepted offer) is no one's activity.
	for user := range out {
		if !strings.HasPrefix(user, "usr_") {
			delete(out, user)
		}
	}
	return out
}

// Target is a target for a user (UserID nil: everyone without their own) for
// a week or month.
type Target struct {
	UserID *string
	Period string
	Goal   Goal
}

// TargetFor is the user's own target for the period, else the one for
// everyone; false when neither is set.
func TargetFor(targets []Target, userID, period string) (Goal, bool) {
	var everyone *Goal
	for _, t := range targets {
		if t.Period != period {
			continue
		}
		if t.UserID != nil && *t.UserID == userID {
			return t.Goal, true
		}
		if t.UserID == nil {
			g := t.Goal
			everyone = &g
		}
	}
	if everyone != nil {
		return *everyone, true
	}
	return Goal{}, false
}

// StageWeight is how likely an application at a stage is to be placed (and
// the fee paid), and the month its fee is expected in (months from this one).
type StageWeight struct {
	Stage       string
	Percent     int
	MonthsAhead int
}

// ForecastStages are the stages the forecast counts, in order.
var ForecastStages = []string{"submitted", "interview", "offer", "placed"}

// DefaultWeights are the weights until a company sets its own.
var DefaultWeights = []StageWeight{
	{Stage: "submitted", Percent: 10, MonthsAhead: 2},
	{Stage: "interview", Percent: 30, MonthsAhead: 1},
	{Stage: "offer", Percent: 70, MonthsAhead: 0},
	{Stage: "placed", Percent: 100, MonthsAhead: 0},
}

// Weights are the company's saved weights over the defaults, in stage order.
func Weights(saved []StageWeight) []StageWeight {
	out := append([]StageWeight{}, DefaultWeights...)
	for _, s := range saved {
		for i := range out {
			if out[i].Stage == s.Stage {
				out[i] = s
			}
		}
	}
	return out
}

// CheckSettings checks targets and weights a person saves: one target per
// user (or everyone) and period, each number in range, and known stages.
// activeUsers are the company's users a target may be for; categories are the
// company's stage categories a target may be set for.
func CheckSettings(targets []Target, weights []StageWeight, activeUsers, categories map[string]bool) apperr.FieldErrors {
	var errs apperr.FieldErrors
	seen := map[string]bool{}
	for i, t := range targets {
		at := "targets." + strconv.Itoa(i) + "."
		if t.Period != Week && t.Period != Month {
			errs.Add(at+"period", "must be week or month")
		}
		who := ""
		if t.UserID != nil {
			who = *t.UserID
			if !activeUsers[who] {
				errs.Add(at+"userId", "is not an active user of this company")
			}
		}
		if seen[who+"|"+t.Period] {
			errs.Add(at+"userId", "already has a target for this period")
		}
		seen[who+"|"+t.Period] = true
		names := []string{"calls", "cvsSent", "interviews", "placements"}
		for j, v := range []*int{t.Goal.Calls, t.Goal.CVsSent, t.Goal.Interviews, t.Goal.Placements} {
			if v != nil && (*v < 0 || *v > 100000) {
				errs.Add(at+"goal."+names[j], "between 0 and 100000")
			}
		}
		for key, v := range t.Goal.Categories {
			switch {
			case !categories[key]:
				errs.Add(at+"goal.categories."+key, "is not one of the company's stage categories")
			case v < 0 || v > 100000:
				errs.Add(at+"goal.categories."+key, "between 0 and 100000")
			}
		}
	}
	stages := map[string]bool{}
	for i, w := range weights {
		at := "stageWeights." + strconv.Itoa(i) + "."
		known := false
		for _, s := range ForecastStages {
			known = known || s == w.Stage
		}
		switch {
		case !known:
			errs.Add(at+"stage", "must be submitted, interview, offer or placed")
		case stages[w.Stage]:
			errs.Add(at+"stage", "is given twice")
		}
		stages[w.Stage] = true
		if w.Percent < 0 || w.Percent > 100 {
			errs.Add(at+"percent", "between 0 and 100")
		}
		if w.MonthsAhead < 0 || w.MonthsAhead > 12 {
			errs.Add(at+"monthsAhead", "between 0 and 12")
		}
	}
	return errs
}
