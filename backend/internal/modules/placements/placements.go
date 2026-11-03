// Package placements records candidates placed in jobs: the start date, the
// guarantee period (leaving within it is a fall-through), the recruiter
// credited, whether the fee is invoiced and paid, and the agreed salary and
// fee.
//
// Recording a placement moves the application to placed, in the same
// transaction. Salary and fee live in their own table: without
// placements.fees:read the database returns no rows for it (row-level
// security), and without placements.fees:update it refuses writes.
//
// Layout (the same in every module):
//
//	placements.go  the module's public API and what it needs from other modules
//	domain.go      the rules, as pure functions on plain values (unit tested)
//	service.go     use cases: one transaction each
//	http.go        HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/placements.sql); private to this module
package placements

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Application is what this module needs to know about an application.
type Application struct {
	Stage                      string
	CandidateID, CandidateName string
	JobID, JobTitle            string
	CompanyName                string
}

// Applications reads applications and moves them to placed (pipeline module).
type Applications interface {
	Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	Place(ctx context.Context, tx pgx.Tx, id string) error
}

// Users checks and names the recruiters credited (identity module).
type Users interface {
	ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error)
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Module is the placements module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, applications Applications, users Users) *Module {
	svc := &Service{pool: pool, applications: applications, users: users}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Fact is a placement as reports count it. FeeAmount is nil when none is
// recorded or the user may not see fees (the database leaves them out).
type Fact struct {
	RecruiterID            *string
	Status                 string
	FeeAmount, FeeCurrency *string
}

// Facts returns the placements starting between two days (inclusive), in the
// caller's transaction.
func (m *Module) Facts(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]Fact, error) {
	rows, err := store.New(tx).PlacementFacts(ctx, store.PlacementFactsParams{FromDay: fromDay, ToDay: toDay})
	if err != nil {
		return nil, err
	}
	out := make([]Fact, len(rows))
	for i, r := range rows {
		fee, err := database.DecimalString(r.FeeAmount)
		if err != nil {
			return nil, err
		}
		out[i] = Fact{RecruiterID: r.RecruiterID, Status: r.Status, FeeAmount: fee, FeeCurrency: r.FeeCurrency}
	}
	return out, nil
}
