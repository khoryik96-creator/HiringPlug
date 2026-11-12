// Package automations runs When · If · Then rules: WHEN something happens
// to an application, an interview or a placement (from the audit log, like
// webhooks), IF the job is at a given client, THEN after some days (or
// before a date) email the candidate, the client or the recruiter from the
// recruiter's mailbox, or give the recruiter a task. Emails go on their own
// or wait for a recruiter to approve them. Each rule runs at most once per
// record; when a run is due it checks again that it still applies.
//
// Layout (the same in every module):
//
//	automations.go  the module's public API and what it needs from other modules
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      the automations and their default: one transaction each
//	runs.go         events, runs (the core queue's jobs), approvals and the log
//	tasks.go        the recruiters' tasks
//	http.go         HTTP handlers: contract types <-> service calls
//	internal/store  generated SQL (db/queries/automations.sql); private to this module
package automations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Application is what this module needs to know about an application.
type Application struct {
	Stage                      string
	CandidateID, CandidateName string
	JobID, JobTitle            string
	CompanyID, CompanyName     string
	RecruiterID                *string // the job's owner
}

// Interview is what this module needs to know about an interview.
type Interview struct {
	ApplicationID string
	StartsAt      time.Time
	Status        string // scheduled, completed, cancelled, no_show
}

// Placement is what this module needs to know about a placement.
type Placement struct {
	ApplicationID                   string
	Status                          string // active, fell_through, cancelled
	StartDate, GuaranteeEnds, DueOn time.Time
	Paid                            bool
	RecruiterID                     *string // credited
}

// Email is an email an automation sends (emails module).
type Email struct {
	CandidateID, ApplicationID string
	InterviewID                *string
	To                         string // candidate, client or sender
	Subject, Body              string
	TimeZone                   *time.Location
}

// Deps is what the module needs from other modules, each read in the
// caller's transaction.
type Deps struct {
	Applications interface {
		Applications(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	}
	Interviews interface {
		Interview(ctx context.Context, tx pgx.Tx, id string) (Interview, bool, error)
	}
	Placements interface {
		Placement(ctx context.Context, tx pgx.Tx, id string) (Placement, bool, error)
	}
	Companies interface {
		Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	// Users acts as a user: their permissions and mailbox (identity
	// module); false when they are no longer active.
	Users interface {
		PrincipalFor(ctx context.Context, tx pgx.Tx, userID string) (authz.Principal, bool, error)
	}
	// Mailer sends an email as sender (emails module); an apperr says why
	// it cannot go (no address, do not contact, ...).
	Mailer interface {
		SendAutomated(ctx context.Context, tx pgx.Tx, sender authz.Principal, e Email) error
	}
	TimeZone *time.Location
}

// Module is the automations module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps) *Module {
	if deps.TimeZone == nil {
		deps.TimeZone = time.UTC
	}
	svc := &Service{pool: pool, deps: deps, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindAutomationEvent: m.service.onEvent, api.JobKindAutomationRun: m.service.run}
}
