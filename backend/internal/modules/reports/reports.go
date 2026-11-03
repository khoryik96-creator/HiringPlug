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
}

// Module is the reports module.
type Module struct {
	HTTP *HTTP
}

// New wires the module. Days start and end in timeZone (the company's).
func New(pool *pgxpool.Pool, deps Deps, timeZone *time.Location) *Module {
	return &Module{HTTP: &HTTP{svc: &Service{pool: pool, deps: deps, loc: timeZone}}}
}
