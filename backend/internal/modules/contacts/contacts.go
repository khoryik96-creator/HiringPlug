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
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mailcheck"
)

// Companies is what this module needs from the companies module.
type Companies interface {
	Exists(ctx context.Context, tx pgx.Tx, companyID string) (bool, error)
	Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	LinkedinURL(ctx context.Context, tx pgx.Tx, companyID string) (string, error)
	// Touch notes that someone did something with the company (one of its people).
	Touch(ctx context.Context, tx pgx.Tx, companyID string) error
}

// Roles checks a role against Settings' contact roles (settings module).
type Roles interface {
	HasOption(ctx context.Context, tx pgx.Tx, list, key string) (bool, error)
	// Labels gives the words for the company's lists (exports show a role's words).
	Labels(ctx context.Context, tx pgx.Tx) (map[string]map[string]string, error)
}

// MailServers looks up where a domain receives mail. *net.Resolver does;
// tests pass a fake.
type MailServers interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Mailboxes asks mail servers whether a mailbox exists (platform/mailcheck;
// tests pass a fake that needs no network).
type Mailboxes interface {
	Check(ctx context.Context, email, mode, controlID string) (mailcheck.Result, error)
}

// Module is the contacts module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. files keeps LinkedIn PDFs until they are read.
func New(pool *pgxpool.Pool, companies Companies, roles Roles, mail MailServers, mailboxes Mailboxes, files *blob.Store, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, companies: companies, roles: roles, mail: mail, mailboxes: mailboxes, files: files}
	return &Module{service: svc, HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindContactLinkedinRead: m.service.saveLinkedinRead, api.JobKindContactCheckMailbox: m.service.runMailboxCheck}
}

// Exists reports whether a contact exists in tx's tenant.
func (m *Module) Exists(ctx context.Context, tx pgx.Tx, contactID string) (bool, error) {
	return m.service.Exists(ctx, tx, contactID)
}

// Brief is what other modules need to show a contact.
type Brief struct {
	Name          string
	CompanyID     string
	Email         *string
	Phone, Mobile *string
}

// Briefs returns name and company of the given contacts (missing ones are
// left out), for other modules (e.g. a job's hiring contact).
func (m *Module) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	return m.service.Briefs(ctx, tx, ids)
}

// CountWithRole returns how many contacts have a role (e.g. before Settings
// removes it), in the caller's transaction.
func (m *Module) CountWithRole(ctx context.Context, tx pgx.Tx, role string) (int, error) {
	return m.service.CountWithRole(ctx, tx, role)
}

func (m *Module) LockInvoiceRecipient(ctx context.Context, tx pgx.Tx, id, companyID, address string) (bool, error) {
	_, err := store.New(tx).LockInvoiceRecipient(ctx, store.LockInvoiceRecipientParams{ID: id, CompanyID: companyID, Address: address})
	if database.IsNoRows(err) {
		return false, nil
	}
	return err == nil, err
}

// UseTimeZone sets the agency's time zone (dates in exports).
func (m *Module) UseTimeZone(loc *time.Location) { m.service.zone = loc }

// Touch notes, in the caller's transaction, that someone did something with
// the contact (a note on them, an email to them): their company too. A
// contact nobody touched for a year shows as stale (00206).
func (m *Module) Touch(ctx context.Context, tx pgx.Tx, contactID string) error {
	return m.service.touch(ctx, tx, contactID)
}

// Untouched counts the contacts nobody touched since then, in the caller's transaction.
func (m *Module) Untouched(ctx context.Context, tx pgx.Tx, since time.Time) (int, error) {
	n, err := store.New(tx).CountUntouchedContacts(ctx, since)
	return int(n), err
}
