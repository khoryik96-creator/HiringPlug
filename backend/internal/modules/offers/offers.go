// Package offers records the terms offered to a candidate for a job (salary,
// start date, expiry, other terms), their approval, and the answer.
//
// Those who may approve offers send them straight away (sending is their
// approval); others ask for approval, and someone else approves the offer or
// sends it back. Changing an offer waiting for approval or approved sends it
// back to draft. Making an offer moves the application to the offer stage.
// The salary lives in its own table: without offers.salary:read the database
// returns no rows for it (row-level security).
//
// Layout (the same in every module):
//
//	offers.go   the module's public API and what it needs from other modules
//	domain.go   the rules, as pure functions on plain values (unit tested)
//	service.go  use cases: one transaction each
//	http.go     HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/offers.sql); private to this module
package offers

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Application is what this module needs to know about an application.
type Application struct {
	Stage                      string
	CandidateID, CandidateName string
	JobID, JobTitle            string
	CompanyName                string
}

// Applications reads applications and moves them to the offer stage (pipeline module).
type Applications interface {
	Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	ReachOffer(ctx context.Context, tx pgx.Tx, id string) error
}

// Users names the people who asked for and gave approval (identity module).
type Users interface {
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Module is the offers module.
type Module struct {
	HTTP *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, applications Applications, users Users) *Module {
	return &Module{HTTP: &HTTP{svc: &Service{pool: pool, applications: applications, users: users}}}
}
