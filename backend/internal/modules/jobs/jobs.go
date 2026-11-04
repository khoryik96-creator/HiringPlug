// Package jobs manages job orders (agency) or requisitions (in-house): what
// is being hired for, at which company, and its salary budget, which only
// permitted roles can see (enforced by the database too).
//
// Layout (the same in every module):
//
//	jobs.go         the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases: one transaction each, rules + storage + audit + visibility
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/jobs.sql); private to this module
//
// The table is job_order, because "job" is the background job queue.
package jobs

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Companies is what this module needs from the companies module.
type Companies interface {
	Exists(ctx context.Context, tx pgx.Tx, companyID string) (bool, error)
	Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// ContactInfo is what a job shows of its hiring contact.
type ContactInfo struct {
	Name      string
	CompanyID string
}

// Contacts is what this module needs from the contacts module (adapted in
// internal/app, so the modules stay independent).
type Contacts interface {
	Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]ContactInfo, error)
}

// Users is what this module needs from the identity module: owners' names,
// and whether someone is an active user (to own jobs).
type Users interface {
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error)
}

// Module is the jobs module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, companies Companies, contacts Contacts, users Users) *Module {
	svc := &Service{pool: pool, companies: companies, contacts: contacts, users: users}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Exists reports whether a job exists in tx's tenant.
func (m *Module) Exists(ctx context.Context, tx pgx.Tx, jobID string) (bool, error) {
	return m.service.Exists(ctx, tx, jobID)
}

// ViewIn returns a job as the user may see it (salary only with permission),
// in the caller's transaction, for other modules (e.g. the AI assistant).
func (m *Module) ViewIn(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (View, error) {
	row, err := getRow(ctx, store.New(tx), id)
	if err != nil {
		return View{}, err
	}
	return m.service.view(ctx, tx, p, fromRow(row))
}

// Brief is what other modules need to show a job.
type Brief struct {
	Title     string
	CompanyID string
	ContactID *string // the hiring contact
	CreatedAt time.Time
}

// Briefs returns title and company of the given jobs (missing ones are left
// out), for other modules (e.g. a candidate's applications).
func (m *Module) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	return m.service.Briefs(ctx, tx, ids)
}

// OpenJob is an open job, for other modules (e.g. the dashboard).
type OpenJob struct {
	ID, Title, CompanyID string
}

// Open returns the newest open jobs, at most limit.
func (m *Module) Open(ctx context.Context, tx pgx.Tx, limit int) ([]OpenJob, error) {
	rows, err := m.service.Open(ctx, tx, limit)
	return rows, err
}
