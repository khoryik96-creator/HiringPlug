// Package leads collects companies that are hiring, as business development
// leads: from their public job board feeds (Greenhouse, Lever, Ashby,
// SmartRecruiters), from careers pages with schema.org JobPosting markup
// (robots.txt respected), and from ads saved by hand. Sites whose terms
// forbid collecting (LinkedIn, JobStreet) are never read.
//
// Each source is checked when added and then every day by the Go worker:
// each opening becomes a lead, one listed again is updated (and reopened),
// and one no longer listed is marked closed. Fetches go through
// platform/safehttp, so an address cannot reach the server's own network.
//
// Layout (the same in every module):
//
//	leads.go    the module's public API and what it needs from other modules
//	domain.go   the rules, as pure functions on plain values (unit tested)
//	parse.go    reading openings from feeds and careers pages (unit tested)
//	service.go  use cases: one transaction each
//	jobs.go     lead_source.check: fetching a source and updating its leads
//	http.go     HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/leads.sql); private to this module
package leads

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Companies checks the companies leads are linked to (companies module).
type Companies interface {
	Exists(ctx context.Context, tx pgx.Tx, id string) (bool, error)
}

// Module is the leads module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. allowPrivate lets careers pages be on private
// networks (local testing only).
func New(pool *pgxpool.Pool, companies Companies, allowPrivate bool) *Module {
	svc := &Service{pool: pool, companies: companies, allowPrivate: allowPrivate}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindLeadSourceCheck: m.service.check}
}
