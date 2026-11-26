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
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
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
		CheckFormattedCapacity(ctx context.Context, tx pgx.Tx, candidateID string, replace []string) error
		AddFormatted(ctx context.Context, tx pgx.Tx, tenantID, actorID, candidateID, fileName string, data []byte, replace []string) (string, func(), error)
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
		// NewestCV is the candidate's newest CV they sent of these content
		// types (their CV first); "" when none.
		NewestCV(ctx context.Context, tx pgx.Tx, candidateID string, contentTypes []string) (string, error)
	}
	// Files reads a document's file, to check it before it goes to a client
	// (documents module; NOT_FOUND when the user may not see it).
	Files interface {
		FormattedFile(ctx context.Context, tx pgx.Tx, documentID string) (FormattedFile, error)
	}
	// AI is the company's AI provider, if it has one and allowance left this
	// month, and the company's AI use, where each run's tokens are recorded
	// (aiusage).
	AI interface {
		Allowed(ctx context.Context, tx pgx.Tx, tenantID, feature string) (*api.AiAccess, bool, error)
		RecordAI(ctx context.Context, tx pgx.Tx, tenantID, feature, sourceID, runKey string, userID *string, input, output int64) error
	}
}

// Module is the cvformats module.
type Module struct {
	HTTP *HTTP
	svc  *Service
}

// Finished is a CV Studio run that is done or failed.
type Finished struct {
	ID, CandidateID string
	Failed, Blind   bool
	At              time.Time
}

// FinishedBy is the CV Studio runs userID started that finished since then,
// newest first, for their notifications (in tx: candidates they may not see
// are left out).
func (m *Module) FinishedBy(ctx context.Context, tx pgx.Tx, userID string, since time.Time) ([]Finished, error) {
	rows, err := store.New(tx).StudioCvsFinishedBy(ctx, store.StudioCvsFinishedByParams{UserID: userID, Since: &since})
	out := make([]Finished, 0, len(rows))
	for _, r := range rows {
		if r.FinishedAt == nil {
			continue
		}
		out = append(out, Finished{ID: r.ID, CandidateID: r.CandidateID, Failed: r.Status == "failed", Blind: r.Blind, At: *r.FinishedAt})
	}
	return out, err
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindCvStudioFormatted: m.svc.saveStudio}
}

// New wires the module. Templates are kept in files, up to maxUploadBytes.
func New(pool *pgxpool.Pool, deps Deps, files *blob.Store, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, deps: deps, files: files}
	return &Module{HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}, svc: svc}
}
