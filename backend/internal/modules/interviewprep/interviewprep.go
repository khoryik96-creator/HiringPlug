// Package interviewprep is a job's interview insights: the questions asked
// at its interviews (as candidates and the client tell them), kept with the
// job and archived once no longer asked, and the prep material the AI writes
// from the job, the client and those questions: soft skills and technical
// questions, each with why it is asked, how to approach it, a few example
// phrasings, the trade-offs and what to avoid, in short points. Salary never
// goes to the AI.
//
// Layout (the same in every module):
//
//	interviewprep.go  the module's public API: what internal/app and other modules may use
//	domain.go         the rules, as pure functions on plain values (unit tested)
//	service.go        the questions: one transaction each, rules + storage + audit
//	prep.go           the AI's prep material: queued, written, listed
//	http.go           HTTP handlers: contract types <-> service calls; no business rules
//	internal/store    generated SQL (db/queries/interview_prep.sql); private to this module
package interviewprep

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Job is what the AI is told of a job (never its salary).
type Job struct {
	Title, CompanyID, CompanyName string
	Location, WorkArrangement     *string
	Description, AdText           string
}

// Company is what the AI is told of the client.
type Company struct{ Industry, About *string }

// Deps are the other modules this one needs, passed in by internal/app.
type Deps struct {
	// Jobs: whether a job exists in the company, and what the AI is told of
	// it (a NotFound error when there is no such job).
	Jobs interface {
		Exists(ctx context.Context, tx pgx.Tx, jobID string) (bool, error)
		ForInterviewPrep(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (Job, error)
	}
	Companies interface {
		ForInterviewPrep(ctx context.Context, tx pgx.Tx, companyID string) (Company, error)
	}
	// AI is the company's AI provider, if it has one and allowance left this
	// month, and the company's AI use, where each run's tokens are recorded
	// (aiusage).
	AI interface {
		Allowed(ctx context.Context, tx pgx.Tx, tenantID string) (*api.AiAccess, bool, error)
		RecordAI(ctx context.Context, tx pgx.Tx, tenantID, feature, sourceID, runKey string, userID *string, input, output int64) error
	}
}

// Module is the interview_prep module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module; zone is where the company works (its "today").
func New(pool *pgxpool.Pool, deps Deps, zone *time.Location) *Module {
	if zone == nil {
		zone = time.UTC
	}
	svc := &Service{pool: pool, deps: deps, now: time.Now, zone: zone}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers are the module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindInterviewPrepWritten: m.service.written}
}
