// Package reports works out how the business is doing over a period: how far
// candidates get (pipeline conversion), which sources bring candidates who
// get hired, placements and fees per recruiter, and how long jobs take to
// fill.
//
// It keeps no data of its own: it reads other modules' records through their
// public functions, in one transaction, as the user (fees the user may not
// see are left out by the database).
//
// Layout:
//
//	reports.go  the module's public API and what it needs from other modules
//	domain.go   the calculations, as pure functions on plain values (unit tested)
//	service.go  the report: one read-only transaction
//	http.go     HTTP handler: contract types <-> service calls
package reports

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Application is an application made in the period (pipeline module).
type Application struct {
	CandidateID string
	Channel     *string
	Stages      []string
}

// Placed is an application's first move to placed (pipeline module).
type Placed struct {
	JobID               string
	CreatedAt, PlacedAt time.Time
}

// Job is what reports need to know about a job (jobs module).
type Job struct {
	Title  string
	Opened time.Time
}

// Deps is what the module reads from other modules, in its transaction.
type Deps struct {
	Applications interface {
		Facts(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Application, error)
		FirstPlaced(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Placed, error)
	}
	// Sources returns where each candidate came from (candidates module).
	Sources interface {
		Sources(ctx context.Context, tx pgx.Tx, ids []string) (map[string]*string, error)
	}
	Jobs interface {
		Jobs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Job, error)
	}
	Placements interface {
		Facts(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]Placement, error)
	}
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	// Activity reads what each user did (candidates, notes, pipeline,
	// emails, interviews and placements modules).
	Activity ActivitySources
	// Forecast reads the open pipeline and what each placement would bring
	// (pipeline, placements, jobs and companies modules).
	Forecast ForecastSources
}

// ActivitySources read, in the caller's transaction, what users did in a
// period: [from, to) as instants, or fromDay..toDay as days.
type ActivitySources interface {
	ScreeningCalls(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]ScreeningCall, error)
	CallNotes(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CallNote, error)
	// ScreeningCallNotes returns those of some notes written with a screening call.
	ScreeningCallNotes(ctx context.Context, tx pgx.Tx, noteIDs []string) ([]string, error)
	Submitted(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CVSent, error)
	CVsEmailed(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CVSent, error)
	Interviews(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Done, error)
	Placements(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Done, error)
	// Members are the company's active users.
	Members(ctx context.Context, tx pgx.Tx) ([]Member, error)
}

// ForecastSources read, in the caller's transaction (as the user: fees and
// salary budgets they may not see are left out by the database), what the
// forecast needs.
type ForecastSources interface {
	AtStages(ctx context.Context, tx pgx.Tx, stages []string) ([]OpenApplication, error)
	ToInvoice(ctx context.Context, tx pgx.Tx) ([]Uninvoiced, error)
	// WithPlacement reports which of some applications have a placement.
	WithPlacement(ctx context.Context, tx pgx.Tx, applicationIDs []string) (map[string]bool, error)
	Jobs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]FeeJob, error)
	Terms(ctx context.Context, tx pgx.Tx, companyID string) (Terms, error)
}

// ScreeningCall is a screening call made by a user on a day, with the note
// written with it (nil when none).
type ScreeningCall struct {
	By     string
	Day    time.Time
	NoteID *string
}

// CallNote is a call logged as a note: kind screening, reference or call.
type CallNote struct {
	ID, Kind, By string
	At           time.Time
}

// CVSent is a candidate's CV sent to a client by a user: an application
// moved to submitted, or a CV attached to an email.
type CVSent struct {
	By, CandidateID string
	At              time.Time
}

// Done is something a user did at a time (an interview arranged, a
// placement recorded).
type Done struct {
	By string
	At time.Time
}

// Member is an active user.
type Member struct{ ID, Name string }

// OpenApplication is an application at a forecast stage.
type OpenApplication struct {
	ID, Stage, JobID, JobTitle, CompanyName, CandidateName string
}

// Uninvoiced is a placement that stands and is not yet invoiced.
type Uninvoiced struct {
	ApplicationID          string
	RecruiterID            *string
	StartDate              time.Time
	FeeAmount, FeeCurrency *string // nil when not recorded (or hidden)
}

// FeeJob is what the forecast needs of a job: its client, owner and salary
// budget (nil when none, or hidden).
type FeeJob struct {
	CompanyID string
	OwnerID   *string
	Budget    *Budget
}

// Module is the reports module.
type Module struct {
	HTTP *HTTP
}

// New wires the module. Days start and end in timeZone (the company's).
func New(pool *pgxpool.Pool, deps Deps, timeZone *time.Location) *Module {
	return &Module{HTTP: &HTTP{svc: &Service{pool: pool, deps: deps, loc: timeZone}}}
}
