// Package companies manages companies: clients and prospects (agencies) or
// your own organisation's departments (in-house teams).
//
// Layout (the same in every module):
//
//	companies.go     the module's public API: what internal/app and other modules may use
//	domain.go     the rules, as pure functions on plain values (unit tested)
//	service.go    use cases: one transaction each, rules + storage + audit
//	http.go       HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/companies.sql); private to this module
package companies

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is the companies module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool) *Module {
	svc := &Service{pool: pool}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Exists reports whether a company exists in tx's tenant. Other modules (e.g.
// contacts, jobs) use it to check references inside their own transaction.
func (m *Module) Exists(ctx context.Context, tx pgx.Tx, companyID string) (bool, error) {
	return m.service.Exists(ctx, tx, companyID)
}

// Names returns the names of the given companies (missing ones are left out),
// for other modules that show a company next to their records.
func (m *Module) Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	return m.service.Names(ctx, tx, ids)
}
