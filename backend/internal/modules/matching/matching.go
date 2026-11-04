// Package matching scores how well a candidate fits a job they are on (an
// application): a score with reasons, each quoting the job and the CV, read
// in the background by the AI provider the deployment chose (through the
// Python AI worker).
//
// What the AI reads is gathered here: the job's text and the CV's text, with
// names, contact details and salary taken out. Its answer is checked
// against what it was sent (platform/quote): a requirement is kept only if it
// quotes the job, and counts as met only with evidence quoting the CV. The
// score is worked out here from what is left, never taken from the AI.
//
// Layout (the same in every module):
//
//	matching.go     the module's public API and what it needs from other modules
//	domain.go       the texts sent, the check and the score (unit tested)
//	service.go      use cases: one transaction each
//	jobs.go         the handler for match.scored on the core queue
//	http.go         HTTP handlers: contract types <-> service calls
//	internal/store  generated SQL (db/queries/matching.sql); private to this module
package matching

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/matching/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Deps is what the module needs from other modules, in the caller's
// transaction. Each returns a NotFound error for a record that does not
// exist (or that the user may not see).
type Deps struct {
	Applications interface {
		ForMatching(ctx context.Context, tx pgx.Tx, id string) (Application, error)
		// OpenOnJob returns the job's applications that are not rejected or withdrawn.
		OpenOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]Application, error)
		// OpenOfCandidate returns the candidate's applications that are not rejected or withdrawn.
		OpenOfCandidate(ctx context.Context, tx pgx.Tx, candidateID string) ([]Application, error)
		// CandidatesOnJob returns the candidates on a job, at any stage.
		CandidatesOnJob(ctx context.Context, tx pgx.Tx, jobID string) ([]string, error)
	}
	Candidates interface {
		ForMatching(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Candidate, error)
		// Briefs returns the given candidates the user may see (others are left out).
		Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]CandidateBrief, error)
	}
	// CVs returns the text of the candidate's newest CV ("" if none), and
	// finds CVs sharing words with a text (possible candidates).
	CVs interface {
		LatestCVText(ctx context.Context, tx pgx.Tx, candidateID string) (string, error)
		SearchCVs(ctx context.Context, tx pgx.Tx, text string, exclude []string, limit int) ([]CVHit, error)
	}
	// Consent says whether a candidate withdrew consent (privacy module):
	// their data is then not sent to the AI or suggested. Nil: nobody did.
	Consent interface {
		DoNotContact(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
	}
	Jobs interface {
		ForMatching(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Job, error)
	}
	// OtherAIUse is the company's other use of the AI (the assistant): one
	// monthly allowance covers both. Nil: none.
	OtherAIUse interface {
		TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error)
	}
}

// Settings is how matching may be used.
type Settings struct {
	Enabled       bool
	MonthlyTokens int64
	TimeZone      *time.Location // months start on the 1st here
}

// Module is the matching module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps, settings Settings) *Module {
	if settings.TimeZone == nil {
		settings.TimeZone = time.UTC
	}
	svc := &Service{pool: pool, deps: deps, settings: settings, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindMatchScored: m.service.scored}
}

// TokensSince returns the AI tokens the company used for matching since the
// given time, in the caller's transaction (the assistant counts them in the
// shared allowance).
func (m *Module) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).TokensSince(ctx, since)
}
