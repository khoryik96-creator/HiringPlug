// Package bin is the Bin: candidates, contacts and companies an admin
// deleted. A binned record is hidden by the database (a restrictive policy on
// binned_at), waits 30 days here, can be restored, and is then deleted for
// good with what belongs to it, by each owning module. A company's contacts
// and jobs go with it unless moved to another company first. A record with a
// placement is never binned. The modules that own the records are passed in
// by internal/app through the small interfaces below.
//
// Layout (the same in every module):
//
//	bin.go        the module's public API: what internal/app and other modules may use
//	domain.go     the rules, as pure functions on plain values (unit tested)
//	service.go    use cases: one transaction each, rules + storage + audit
//	purge.go      the bin.purge job: deleting for good after 30 days
//	http.go       HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/bin.sql); private to this module
package bin

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Candidates is the candidates module.
type Candidates interface {
	Label(ctx context.Context, tx pgx.Tx, id string) (string, error)
	Bin(ctx context.Context, tx pgx.Tx, id string) (string, error)
	Restore(ctx context.Context, tx pgx.Tx, id string) error // ErrTaken when another has its email
	Binned(ctx context.Context, tx pgx.Tx, ids []string) ([]string, error)
	Purge(ctx context.Context, tx pgx.Tx, id string) error
}

// Contacts is the contacts module.
type Contacts interface {
	Label(ctx context.Context, tx pgx.Tx, id string) (string, error)
	Bin(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	Restore(ctx context.Context, tx pgx.Tx, ids []string) error // ErrTaken when another has an email
	IDsOfCompany(ctx context.Context, tx pgx.Tx, companyID string) ([]string, error)
	MoveToCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error
	Purge(ctx context.Context, tx pgx.Tx, ids []string) error
}

// Companies is the companies module.
type Companies interface {
	Label(ctx context.Context, tx pgx.Tx, id string) (string, error)
	Bin(ctx context.Context, tx pgx.Tx, id string) (string, error)
	Restore(ctx context.Context, tx pgx.Tx, id string) error // ErrTaken when another has its name
	Purge(ctx context.Context, tx pgx.Tx, id string) ([]string, error)
}

// Jobs is the jobs module.
type Jobs interface {
	IDsOfCompany(ctx context.Context, tx pgx.Tx, companyID string) ([]string, error)
	Bin(ctx context.Context, tx pgx.Tx, ids []string) error
	Restore(ctx context.Context, tx pgx.Tx, ids []string) error
	Binned(ctx context.Context, tx pgx.Tx, ids []string) ([]string, error)
	MoveToCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error
	ClearContacts(ctx context.Context, tx pgx.Tx, contactIDs []string) error
	Purge(ctx context.Context, tx pgx.Tx, ids []string) ([]string, error)
}

// Application is one pipeline entry, by candidate and job.
type Application struct{ ID, CandidateID, JobID string }

// Pipeline is the pipeline and placements modules: applications, and which have a placement.
type Pipeline interface {
	ApplicationsOf(ctx context.Context, tx pgx.Tx, candidateIDs, jobIDs []string) ([]Application, error)
	Placed(ctx context.Context, tx pgx.Tx, applicationIDs []string) (int, error)
	SetBinned(ctx context.Context, tx pgx.Tx, ids []string, binned bool) error
	Purge(ctx context.Context, tx pgx.Tx, ids []string) error
}

// Files are the candidates' CVs (documents module), the notes on records
// (notes module) and what the AI said about them (assistant module, deleted
// with a candidate's or job's notes, and with applications).
type Files interface {
	DocumentLabel(ctx context.Context, tx pgx.Tx, id string) (string, error)
	BinActiveDocument(ctx context.Context, tx pgx.Tx, id string) error
	RestoreDocument(ctx context.Context, tx pgx.Tx, id string) error
	PurgeDocument(ctx context.Context, tx pgx.Tx, id string) ([]string, error)
	PurgeCandidate(ctx context.Context, tx pgx.Tx, candidateID string) ([]string, error)
	DeleteNotes(ctx context.Context, tx pgx.Tx, subjectType string, ids []string) error
}

// Taken reports whether a Restore error means another record has the email or name now.
type Taken func(error) bool

// Deps are the modules the Bin works through.
type Deps struct {
	Users interface {
		UserNames(context.Context, pgx.Tx, []string) (map[string]string, error)
	}
	Candidates Candidates
	Contacts   Contacts
	Companies  Companies
	Jobs       Jobs
	Pipeline   Pipeline
	Files      Files
	Taken      Taken
	DeleteFile func(string) error
}

// Module is the bin module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps) *Module {
	svc := &Service{pool: pool, deps: deps}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the handlers for the module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindBinPurge: m.service.purge, api.JobKindBinRemoveFiles: m.service.removeFiles}
}

// RecordDocument keeps a CV replaced by a recruiter for the ordinary Bin retention period.
func (m *Module) RecordDocument(ctx context.Context, tx pgx.Tx, tenantID, id, name, actorID string) error {
	return m.service.recordDocument(ctx, tx, tenantID, id, name, actorID)
}
