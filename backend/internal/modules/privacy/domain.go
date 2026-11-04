package privacy

// The rules, as pure functions: what a consent record and a request must
// hold, when a request is due, and which consent wins when two candidates
// are merged. Nothing here deletes data: people decide every request.

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Consent statuses.
const (
	Given     = "given"
	Withdrawn = "withdrawn"
)

// Request kinds and statuses.
const (
	KindAccess          = "access"
	KindCorrection      = "correction"
	KindWithdrawConsent = "withdraw_consent"
	StatusOpen          = "open"
	StatusDone          = "done"
	StatusDeclined      = "declined"
)

// MethodCareersForm and MethodRequest are how consent is recorded by the app
// itself: on the careers form, or by closing a withdraw-consent request.
const (
	MethodCareersForm = "careers_form"
	MethodRequest     = "request"
)

// answerDays is how long a data user has to answer an access or a
// correction request (PDPA ss. 31 and 35).
const answerDays = 21

// Consent is a candidate's consent as recorded.
type Consent struct {
	Version     int
	Status      string
	Method      string
	Day         time.Time
	Note        *string
	RecordedBy  string
	UpdatedAt   time.Time
	CandidateID string
}

// ConsentInput is what a person records.
type ConsentInput struct {
	Status, Method string
	Day            time.Time
	Note           *string
}

// Request is a request a candidate made about their data.
type Request struct {
	ID, CandidateID, Kind, Status string
	Version                       int
	ReceivedOn                    time.Time
	Details, Outcome, ClosedBy    *string
	ClosedAt                      *time.Time
	CreatedAt                     time.Time
}

// RequestInput is a request as logged.
type RequestInput struct {
	Kind       string
	ReceivedOn time.Time
	Details    *string
}

// Decision closes (or reopens) a request.
type Decision struct {
	Status  string
	Outcome *string
}

// CheckConsent tidies what a person records and checks it: the day may not
// be in the future (in the company's time zone).
func CheckConsent(in ConsentInput, today time.Time) (ConsentInput, error) {
	var errs apperr.FieldErrors
	in.Note = tidy(in.Note)
	if in.Day.After(today) {
		errs.Add("day", "cannot be in the future")
	}
	if in.Note != nil && utf8.RuneCountInString(*in.Note) > 1000 {
		errs.Add("note", "at most 1000 characters")
	}
	return in, errs.Err()
}

// CheckRequest tidies a logged request and checks it.
func CheckRequest(in RequestInput, today time.Time) (RequestInput, error) {
	var errs apperr.FieldErrors
	in.Details = tidy(in.Details)
	if in.ReceivedOn.After(today) {
		errs.Add("receivedOn", "cannot be in the future")
	}
	if in.Details != nil && utf8.RuneCountInString(*in.Details) > 2000 {
		errs.Add("details", "at most 2000 characters")
	}
	return in, errs.Err()
}

// CheckDecision checks a decision: closing a request needs its outcome
// (what was done, or why it was declined).
func CheckDecision(d Decision) (Decision, error) {
	var errs apperr.FieldErrors
	d.Outcome = tidy(d.Outcome)
	if d.Status != StatusOpen && d.Outcome == nil {
		errs.Add("outcome", "say what was done, or why it was declined")
	}
	if d.Outcome != nil && utf8.RuneCountInString(*d.Outcome) > 2000 {
		errs.Add("outcome", "at most 2000 characters")
	}
	return d, errs.Err()
}

// DueOn is when a request must be answered: 21 days after it was received
// for access and correction requests; nil for others (no fixed time).
func DueOn(r Request) *time.Time {
	if r.Kind != KindAccess && r.Kind != KindCorrection {
		return nil
	}
	d := r.ReceivedOn.AddDate(0, 0, answerDays)
	return &d
}

// Overdue reports whether a request is still open after its due day.
func Overdue(r Request, today time.Time) bool {
	due := DueOn(r)
	return r.Status == StatusOpen && due != nil && today.After(*due)
}

// DaysOpen is the number of days from receiving a request to today (or to
// the day it was closed), never negative.
func DaysOpen(r Request, today time.Time, loc *time.Location) int {
	end := today
	if r.ClosedAt != nil {
		end = Day(*r.ClosedAt, loc)
	}
	return max(int(end.Sub(r.ReceivedOn).Hours()/24), 0)
}

// Merged is the consent a merged candidate keeps when both had one:
// withdrawn wins (if either asked not to be contacted, they are not), else
// the one kept is the one already on the candidate who stays. changed is
// false when nothing on the staying candidate changes.
func Merged(moving Consent, staying *Consent) (out Consent, changed bool) {
	if staying == nil {
		return moving, true
	}
	if moving.Status == Withdrawn && staying.Status != Withdrawn {
		return moving, true
	}
	return *staying, false
}

// Day is the calendar day t falls on in loc, as midnight UTC (how dates are
// stored and compared).
func Day(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Field, Item and Section are a copy of a candidate's data, in plain words:
// what an access request is answered with.
type (
	Field   struct{ Label, Value string }
	Item    []Field
	Section struct {
		Title string
		Items []Item
	}
)

// Held is what other modules hold about a candidate, as the user may see it.
type Held struct {
	Name        string
	Sections    []Section
	NotIncluded []string // what is held but the user may not see
}

func tidy(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}
