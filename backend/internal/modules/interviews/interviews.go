// Package interviews schedules interviews for an application and collects
// each interviewer's feedback.
//
// Scheduling moves the application to the interview stage when it is at an
// earlier one, in the same transaction. An interviewer booked at the same time
// is a clash (refused unless asked to ignore it). Interviewers see each
// other's feedback only after giving their own, so the first opinion does not
// steer the rest; those who schedule see all of it.
//
// Layout (the same in every module):
//
//	interviews.go  the module's public API and what it needs from other modules
//	domain.go      the rules, as pure functions on plain values (unit tested)
//	calendar.go    the interview as an .ics calendar file
//	service.go     use cases: one transaction each
//	http.go        HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/interviews.sql); private to this module
package interviews

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Application is what this module needs to know about an application.
type Application struct {
	ID, Stage                  string
	JobID, JobTitle            string
	CandidateID, CandidateName string
}

// Applications reads applications and moves them to the interview stage (pipeline module).
type Applications interface {
	Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	ReachInterview(ctx context.Context, tx pgx.Tx, id string) error
}

// Users checks interviewers and names them (identity module).
type Users interface {
	ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error)
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Module is the interviews module.
type Module struct {
	HTTP *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, applications Applications, users Users) *Module {
	svc := &Service{pool: pool, applications: applications, users: users}
	return &Module{HTTP: &HTTP{svc: svc}}
}
