// Package cvformats makes formatted CVs: the candidate's profile, work
// history and education filled into a Word template (the built-in standard
// one, or the company's own), saved as the candidate's formatted CV.
//
// Contact details are left out unless asked (so the client goes through the
// recruiter), and salary is included only when asked by someone who may see
// it. Templates are ordinary .docx files with {{placeholders}} (docx.go); an
// upload using unknown ones is refused, saying which.
//
// Layout (the same in every module):
//
//	cvformats.go  the module's public API and what it needs from other modules
//	docx.go       filling Word templates (unit tested)
//	standard.go   the built-in template
//	domain.go     what fills a template, from a candidate (unit tested)
//	service.go    use cases: one transaction each
//	http.go       HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/cvformats.sql); private to this module
package cvformats

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Application is the job a CV is made for (pipeline module).
type Application struct {
	CandidateID, JobTitle, CompanyName string
}

// Deps is what the module needs from other modules, each in the caller's
// transaction.
type Deps struct {
	// Candidates returns a candidate as the user may see them (candidates module).
	Candidates interface {
		ForCV(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Candidate, error)
	}
	Applications interface {
		Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	}
	// Documents saves the result as the candidate's formatted CV (documents module).
	Documents interface {
		AddFormatted(ctx context.Context, tx pgx.Tx, tenantID, candidateID, fileName string, data []byte) (string, func(), error)
	}
	Tenants interface {
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	// SourceCVs gives a CV to format CV Studio's way and a signed link to it (documents module).
	SourceCVs interface {
		SourceCV(ctx context.Context, tx pgx.Tx, id string) (SourceCV, string, error)
	}
	// AI is the company's AI provider, if it has one and allowance left this month.
	AI interface {
		Allowed(ctx context.Context, tx pgx.Tx) (*api.AiAccess, bool, error)
	}
}

// Module is the cvformats module.
type Module struct {
	HTTP *HTTP
	svc  *Service
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindCvStudioFormatted: m.svc.saveStudio}
}

// TokensSince is the AI tokens studio CVs used since a time (the company's
// monthly allowance), in the caller's transaction.
func (m *Module) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return m.svc.TokensSince(ctx, tx, since)
}

// New wires the module. Templates are kept in files, up to maxUploadBytes.
func New(pool *pgxpool.Pool, deps Deps, files *blob.Store, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, deps: deps, files: files}
	return &Module{HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}, svc: svc}
}
