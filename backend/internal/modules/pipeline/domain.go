package pipeline

// The pipeline rules: which stage moves are allowed, as pure functions. Every
// rule lives here, once, and is tested move by move.

import (
	"fmt"
	"slices"
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
	Step           *string // the job's own stage it is in (Step.Key), when the job has custom stages
	Category       *string // the category its stage falls under; nil when closed
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
	JobTitle              string
	CompanyID             string
	CompanyName           string
	CandidateName         string
	CandidateTitle        *string
	CandidateAvailability *string
}

// Move is a request to change an application's stage, and with custom
// stages which of the job's stages (Step, whose kind is To).
type Move struct {
	To     string
	Step   *string
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
	if IsClosed(m.To) {
		m.Step = nil
	}
	if m.To == current.Stage && same(m.Step, current.Step) && same(m.Reason, current.Reason) && same(m.Note, current.ReasonNote) {
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

// Step is one of a job's own stages (a custom stage): a name, and the
// standard stage it is a kind of (reports, automations, offers and
// placements go by the kind).
type Step struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Stage    string `json:"stage"`
	Category string `json:"category,omitempty"` // a category of its kind; none: the kind's first
}

// MaxSteps is the most stages a job or template has.
const MaxSteps = 20

var stageLabels = map[string]string{Applied: "Applied", Screening: "Screening", Submitted: "Submitted", Interview: "Interview", Offer: "Offer", Placed: "Placed"}

// StandardSteps are the stages a job has with no template: one of each kind.
func StandardSteps() []Step {
	out := make([]Step, len(OpenStages))
	for i, st := range OpenStages {
		out[i] = Step{Key: st, Label: stageLabels[st], Stage: st}
	}
	return out
}

func stageIndex(stage string) int {
	for i, st := range OpenStages {
		if st == stage {
			return i
		}
	}
	return -1
}

// NormalizeSteps tidies and checks a list of stages: 1 to 20, each named
// (at most 40 characters, no two alike) and of an open kind, in the order of
// their kinds (all the screening stages after the applied ones, and so on).
// A stage keeps its key; a new one gets newKey's.
func NormalizeSteps(in []Step, newKey func() string) ([]Step, apperr.FieldErrors) {
	var errs apperr.FieldErrors
	switch {
	case len(in) == 0:
		errs.Add("steps", "add at least one stage")
		return nil, errs
	case len(in) > MaxSteps:
		errs.Add("steps", "at most 20 stages")
		return nil, errs
	}
	out := make([]Step, len(in))
	names, keys := map[string]bool{}, map[string]bool{}
	last := 0
	for i, st := range in {
		at := func(field string) string { return fmt.Sprintf("steps.%d.%s", i, field) }
		st.Label = strings.Join(strings.Fields(st.Label), " ")
		switch n := len([]rune(st.Label)); {
		case n == 0:
			errs.Add(at("label"), "enter a name")
		case n > 40:
			errs.Add(at("label"), "at most 40 characters")
		case names[strings.ToLower(st.Label)]:
			errs.Add(at("label"), "another stage has this name")
		}
		names[strings.ToLower(st.Label)] = true
		k := stageIndex(st.Stage)
		switch {
		case k < 0:
			errs.Add(at("stage"), "must be applied, screening, submitted, interview, offer or placed")
		case k < last:
			errs.Add(at("stage"), "comes before the stage above it: keep the stages in order (applied first, placed last)")
		default:
			last = k
		}
		if st.Key == "" || keys[st.Key] {
			st.Key = newKey()
		}
		keys[st.Key] = true
		out[i] = st
	}
	return out, errs
}

// StepFor is the job's stage with key, when it is of kind stage.
func StepFor(steps []Step, key, stage string) (Step, bool) {
	for _, st := range steps {
		if st.Key == key && st.Stage == stage {
			return st, true
		}
	}
	return Step{}, false
}

// SameSteps reports whether two lists of stages are the same.
func SameSteps(a, b []Step) bool { return slices.Equal(a, b) }
