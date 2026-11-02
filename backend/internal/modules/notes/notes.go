// Package notes keeps notes on candidates, jobs, companies and contacts
// (screening, interview, call or general), and serves each record's activity:
// what changed, who changed it and when, read from the audit log. Notes marked
// sensitive (e.g. salary talks) are invisible without notes.sensitive:read,
// enforced by the database too.
//
// Layout (the same in every module):
//
//	notes.go        the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases: one transaction each, rules + storage + audit
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/notes.sql); private to this module
package notes

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exists reports whether a record exists in tx's tenant.
type Exists func(ctx context.Context, tx pgx.Tx, id string) (bool, error)

// Subjects maps a subject type (candidate, job, company, contact) to the
// owning module's check, passed in by internal/app.
type Subjects map[string]Exists

// Users is what this module needs from the identity module.
type Users interface {
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Module is the notes module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, subjects Subjects, users Users) *Module {
	svc := &Service{pool: pool, subjects: subjects, users: users}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}
