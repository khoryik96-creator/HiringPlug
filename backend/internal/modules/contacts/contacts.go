// Package contacts manages people at companies (hiring managers, HR), each
// with a checked email address: well formed, the domain receives mail, and
// not a shared or throwaway mailbox.
//
// Layout (the same in every module):
//
//	contacts.go     the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	emailcheck.go   the email check rules (pure)
//	service.go      use cases: one transaction each, rules + storage + audit
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/contacts.sql); private to this module
package contacts

import (
	"context"
	"net"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Companies is what this module needs from the companies module.
type Companies interface {
	Exists(ctx context.Context, tx pgx.Tx, companyID string) (bool, error)
	Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// MailServers looks up where a domain receives mail. *net.Resolver does;
// tests pass a fake.
type MailServers interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Module is the contacts module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. files keeps LinkedIn PDFs until they are read.
func New(pool *pgxpool.Pool, companies Companies, mail MailServers, files *blob.Store, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, companies: companies, mail: mail, files: files}
	return &Module{service: svc, HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindContactLinkedinRead: m.service.saveLinkedinRead}
}

// Exists reports whether a contact exists in tx's tenant.
func (m *Module) Exists(ctx context.Context, tx pgx.Tx, contactID string) (bool, error) {
	return m.service.Exists(ctx, tx, contactID)
}

// Brief is what other modules need to show a contact.
type Brief struct {
	Name      string
	CompanyID string
	Email     *string
}

// Briefs returns name and company of the given contacts (missing ones are
// left out), for other modules (e.g. a job's hiring contact).
func (m *Module) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	return m.service.Briefs(ctx, tx, ids)
}
