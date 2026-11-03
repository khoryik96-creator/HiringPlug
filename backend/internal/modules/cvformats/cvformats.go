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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
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
}

// Module is the cvformats module.
type Module struct {
	HTTP *HTTP
}

// New wires the module. Templates are kept in files, up to maxUploadBytes.
func New(pool *pgxpool.Pool, deps Deps, files *blob.Store, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, deps: deps, files: files}
	return &Module{HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}
