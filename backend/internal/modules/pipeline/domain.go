package pipeline

// The pipeline rules: which stage moves are allowed, as pure functions. Every
// rule lives here, once, and is tested move by move.

import (
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Stages, in pipeline order. Rejected and withdrawn close an application.
const (
	Applied   = "applied"
	Screening = "screening"
	Submitted = "submitted"
	Interview = "interview"
	Offer     = "offer"
	Placed    = "placed"
	Rejected  = "rejected"
	Withdrawn = "withdrawn"
)

// OpenStages are the stages of an application still in progress, in order.
var OpenStages = []string{Applied, Screening, Submitted, Interview, Offer, Placed}

// IsClosed reports whether stage closes the application.
func IsClosed(stage string) bool { return stage == Rejected || stage == Withdrawn }

// Application is a candidate on a job.
type Application struct {
	ID             string
	Version        int
	JobID          string
	CandidateID    string
	Stage          string
	Reason         *string
	ReasonNote     *string
	Channel        *string // where the application came from; nil when a recruiter added it
	StageChangedAt time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// View adds what is shown with an application: job, company and candidate.
type View struct {
	Application
	JobTitle       string
	CompanyID      string
	CompanyName    string
	CandidateName  string
	CandidateTitle *string
}

// Move is a request to change an application's stage.
type Move struct {
	To     string
	Reason *string
	Note   *string
}

// CheckNewStage checks the stage a candidate is added at: any open stage.
func CheckNewStage(stage string) apperr.FieldErrors {
	var errs apperr.FieldErrors
	if IsClosed(stage) {
		errs.Add("stage", "must be an open stage (applied to placed)")
	}
	return errs
}

// CheckMove applies the pipeline rules to moving an application from its
// current stage. It returns the normalized move, whether anything changes,
// and an error if the move is not allowed:
//   - rejected and withdrawn need a reason; other stages take none;
//   - a closed application is reopened to an open stage before it can be placed;
//   - a placed candidate can only go back to offer (a correction) or be withdrawn.
func CheckMove(current Application, m Move) (Move, bool, error) {
	if m.Note != nil {
		n := strings.TrimSpace(*m.Note)
		m.Note = &n
		if n == "" {
			m.Note = nil
		}
	}
	var errs apperr.FieldErrors
	switch {
	case IsClosed(m.To) && m.Reason == nil:
		errs.Add("reason", "choose why the application is "+m.To)
	case !IsClosed(m.To) && (m.Reason != nil || m.Note != nil):
		errs.Add("reason", "only applies to rejected or withdrawn")
	}
	if err := errs.Err(); err != nil {
		return m, false, err
	}
	if m.To == current.Stage && same(m.Reason, current.Reason) && same(m.Note, current.ReasonNote) {
		return m, false, nil // nothing changes
	}
	switch {
	case IsClosed(current.Stage) && m.To == Placed:
		return m, false, notAllowed("Reopen the application (move it to an open stage) before placing the candidate.")
	case current.Stage == Placed && m.To != Placed && m.To != Offer && m.To != Withdrawn:
		return m, false, notAllowed("A placed candidate can only go back to offer, or be withdrawn.")
	}
	return m, true, nil
}

// MoveNotAllowedError is a move that breaks a pipeline rule; Message says
// which, in words for the user.
type MoveNotAllowedError struct{ Message string }

func (e MoveNotAllowedError) Error() string { return e.Message }

func notAllowed(message string) error { return MoveNotAllowedError{Message: message} }

func same(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
