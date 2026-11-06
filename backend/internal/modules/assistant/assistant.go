// Package assistant is the AI sidebar: questions about a candidate, a job or
// a candidate for a job, answered in the background by the AI provider the
// deployment chose (through the Python AI worker).
//
// What the AI sees is gathered here from what the asking user may see, with
// contact details and names taken out, and never salary. Answers are shown
// only to the person who asked. Each company has a monthly token allowance,
// shared with CV vs job matching.
//
// Layout (the same in every module):
//
//	assistant.go  the module's public API and what it needs from other modules
//	domain.go     what the AI may see, and the rules for a question (unit tested)
//	service.go    use cases: one transaction each
//	http.go       HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/assistant.sql); private to this module
package assistant

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/assistant/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Deps is what the module needs from other modules, each in the caller's
// transaction and as the given user may see it. Each returns a NotFound
// error for a record that does not exist.
type Deps struct {
	Candidates interface {
		ForAssistant(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Candidate, error)
	}
	// CVs returns the text of the candidate's newest CV ("" if none).
	CVs interface {
		LatestCVText(ctx context.Context, tx pgx.Tx, candidateID string) (string, error)
	}
	Jobs interface {
		ForAssistant(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Job, error)
	}
	Applications interface {
		ForAssistant(ctx context.Context, tx pgx.Tx, id string) (Application, error)
	}
	// Market describes who in the company's database does a job's kind of
	// work (counts only: no names, contact details or pay), for market
	// insights. Nil: not given.
	Market interface {
		PoolText(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (string, error)
	}
	// OtherAIUse is the company's other use of the AI (CV vs job matching):
	// one monthly allowance covers both. Nil: none.
	OtherAIUse interface {
		TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error)
	}
}

// Settings is how the assistant may be used.
type Settings struct {
	Enabled       bool
	MonthlyTokens int64
	TimeZone      *time.Location // months start on the 1st here
}

// Module is the assistant module.
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

// TokensSince returns the AI tokens the company used for the assistant since
// the given time, in the caller's transaction (matching counts them in the
// shared allowance).
func (m *Module) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).TokensSince(ctx, since)
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindAssistantAnswered: m.service.answered}
}

// Insight is an answer for other modules (a job's market map).
type Insight struct {
	ID, Status  string
	Text, Model *string
	AskedAt     time.Time
}

// LatestMarketMap returns the user's newest market insights for a job (ok
// is false when they have asked for none), in the caller's transaction.
func (m *Module) LatestMarketMap(ctx context.Context, tx pgx.Tx, jobID string) (in Insight, ok bool, err error) {
	rows, err := store.New(tx).ListAnswers(ctx, store.ListAnswersParams{SubjectType: "job", SubjectID: jobID})
	if err != nil {
		return Insight{}, false, err
	}
	for _, r := range rows {
		if r.Kind == "market_map" {
			return Insight{ID: r.ID, Status: r.Status, Text: r.Answer, Model: r.Model, AskedAt: r.CreatedAt}, true, nil
		}
	}
	return Insight{}, false, nil
}
