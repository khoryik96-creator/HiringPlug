// Package auditlog shows the company's audit log (who changed what, and
// when) and keeps the log of who was shown a candidate's salary.
//
// The audit log itself belongs to the platform (platform/audit writes and
// reads it); this module reads it for the audit log page, names who did
// each thing and what it was about. The salary views (salary_view) are this
// module's own: one line per person, kind, candidate and day in the
// agency's time zone, counted, written only through SalarySeen. Neither log
// ever holds an amount, and nothing is deleted from either.
//
// Layout (the same in every module):
//
//	auditlog.go     the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases: one transaction each
//	export.go       the Excel and CSV downloads
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/auditlog.sql); private to this module
package auditlog

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/auditlog/internal/store"
)

// Names looks up the names of records of one kind (missing ones are left
// out), in the caller's transaction.
type Names interface {
	Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// NamesFunc is a function as Names.
type NamesFunc func(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)

// Names calls f.
func (f NamesFunc) Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	return f(ctx, tx, ids)
}

// Deps is what the module reads from other modules, in its transaction.
type Deps struct {
	// Users names people (identity module).
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	// Keys names API keys (integrations module).
	Keys interface {
		KeyNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	// Records names what an entry is about, by entity type (candidate, job,
	// company, contact, job lead...): the user may see only some of them.
	Records map[string]Names
	// Applications names candidates on jobs (pipeline module), with the
	// candidate the record opens.
	Applications interface {
		Applications(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	}
}

// Application is a candidate on a job as an entry names it: "Siti Aisyah ·
// Accountant", opening the candidate.
type Application struct {
	Name, CandidateID string
}

// ApplicationsFunc is a function as Deps.Applications.
type ApplicationsFunc func(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)

// Applications calls f.
func (f ApplicationsFunc) Applications(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error) {
	return f(ctx, tx, ids)
}

// Config is how the server is set up.
type Config struct {
	// TimeZone is the agency's (HP_TIME_ZONE): a salary view's day, and the
	// days the filters name, are days there.
	TimeZone *time.Location
}

// Module is the auditlog module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps, config Config) *Module {
	if config.TimeZone == nil {
		config.TimeZone = time.UTC
	}
	svc := &Service{pool: pool, deps: deps, loc: config.TimeZone, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// SalarySeen counts one showing of a salary to the transaction's user, in
// the caller's transaction (it satisfies salaryview.Recorder). For a grouped
// kind (a list, search or export: salaryview.Grouped) it is one line a day
// with the candidates it showed; otherwise one line a day for each
// candidate. Nothing is counted for the system. No amount is ever stored.
func (m *Module) SalarySeen(ctx context.Context, tx pgx.Tx, kind string, candidateIDs ...string) error {
	return m.service.record(ctx, tx, kind, candidateIDs)
}

// MoveCandidate moves a merged duplicate's salary views to the kept
// candidate, in the merge's transaction (candidates.RecordMover).
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveSalaryViews(ctx, store.MoveSalaryViewsParams{FromID: fromID, ToID: toID})
}

// UseClock sets the module's clock (tests: a day's boundaries).
func (m *Module) UseClock(now func() time.Time) { m.service.now = now }
