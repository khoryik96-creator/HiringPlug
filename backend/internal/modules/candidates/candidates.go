// Package candidates manages candidate profiles and their salary details.
//
// Layout (the same in every module):
//
//	candidates.go     the module's public API: what internal/app and other modules may use
//	domain.go         the rules, as pure functions on plain values (unit tested)
//	service.go        use cases: one transaction each, rules + storage + audit + visibility
//	http.go           HTTP handlers: contract types <-> service calls; no business rules
//	internal/store    generated SQL (db/queries/candidates.sql); private to this module
//
// Salary details live in their own table. They are only loaded for users with
// candidates.compensation:read, and the database refuses them to anyone else.
package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Module is the candidates module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. phoneRegion is the country assumed for local phone
// numbers, e.g. "MY".
func New(pool *pgxpool.Pool, phoneRegion string) *Module {
	svc := &Service{pool: pool, phoneRegion: phoneRegion}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Exists reports whether a candidate exists in tx's tenant. Other modules use
// it to check references (e.g. a document's candidate) inside their own
// transaction.
func (m *Module) Exists(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error) {
	return m.service.Exists(ctx, tx, candidateID)
}

// Count returns how many candidates the company has, in the caller's
// transaction (the data protection dashboard).
func (m *Module) Count(ctx context.Context, tx pgx.Tx) (int, error) {
	n, err := store.New(tx).CountCandidates(ctx)
	return int(n), err
}

// Brief is what other modules need to show a candidate.
type Brief struct {
	Name         string
	FirstName    string
	Email        *string
	CurrentTitle *string
	Source       *string // where the candidate came from (jobstreet, linkedin, ...)
}

// Briefs returns name and current title of the given candidates (missing ones
// are left out), for other modules (e.g. a job's pipeline).
func (m *Module) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	return m.service.Briefs(ctx, tx, ids)
}

// ViewIn returns a candidate as the user may see them (salary only with
// permission), in the caller's transaction: for formatted CVs.
func (m *Module) ViewIn(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (View, error) {
	q := store.New(tx)
	row, err := q.GetCandidate(ctx, id)
	if database.IsNoRows(err) {
		return View{}, apperr.NotFound("candidate")
	}
	if err != nil {
		return View{}, err
	}
	return loadView(ctx, q, p, fromRow(row))
}
