// Package companies manages companies: clients and prospects (agencies) or
// your own organisation's departments (in-house teams).
//
// Layout (the same in every module):
//
//	companies.go         the module's public API: what internal/app and other modules may use
//	domain.go            the rules, as pure functions on plain values (unit tested)
//	service.go           use cases: one transaction each, rules + storage + audit
//	autofill.go          reading a company's website for details to suggest (unit tested)
//	autofill_service.go  suggestions: reading in the background, accepting, dismissing
//	http.go              HTTP handlers: contract types <-> service calls; no business rules
//	autofill_http.go     HTTP handlers for suggestions
//	internal/store       generated SQL (db/queries/companies.sql); private to this module
package companies

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Module is the companies module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, allowPrivate bool) *Module {
	svc := &Service{pool: pool, allowPrivate: allowPrivate}
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

// WordHits returns, for each word of a job search (numbered as the database's
// app_search_words numbers them), the companies whose name or website has
// it, as "n:companyId", so jobs can be found by their client's name.
func (m *Module) WordHits(ctx context.Context, tx pgx.Tx, query string) ([]string, error) {
	return store.New(tx).CompanyWordHits(ctx, query)
}

// Website returns a company's website ("" when none or no such company),
// for other modules (e.g. blind job ads, which leave it out).
func (m *Module) Website(ctx context.Context, tx pgx.Tx, companyID string) (string, error) {
	w, err := store.New(tx).CompanyWebsite(ctx, companyID)
	if database.IsNoRows(err) {
		return "", nil
	}
	return w, err
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindCompanyAutofill: m.service.autofill}
}
