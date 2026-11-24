// Package assistant is the AI sidebar: questions about a candidate, a job or
// a candidate for a job, answered in the background by the AI provider the
// deployment chose (through the Python AI worker).
//
// What the AI sees is gathered here from what the asking user may see, with
// contact details and names taken out, and never salary. Answers are shown
// only to the person who asked. Each company has one monthly AI limit,
// shared by every AI feature (aiusage), where each answer's tokens are recorded.
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
	// AI is the AI provider the company uses (aiproviders).
	AI interface {
		Access(ctx context.Context, tx pgx.Tx) (*api.AiAccess, bool, error)
	}
	// Agency names the company the sender works for, for emails the AI
	// writes (identity). Nil: none.
	Agency interface {
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	// Usage is the company's AI use and its one monthly limit, shared by
	// every AI feature (aiusage), in the caller's transaction.
	Usage interface {
		// Remaining is this month's allowance left in tokens, work in
		// progress held (0 or less: used up).
		Remaining(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error)
		// ThisMonth is what the company used of this month's limit.
		ThisMonth(ctx context.Context, tx pgx.Tx, tenantID string) (MonthUse, error)
		// RecordAI records one AI run's tokens (feature, the record it was
		// for, which run of it, who asked).
		RecordAI(ctx context.Context, tx pgx.Tx, tenantID, feature, sourceID, runKey string, userID *string, input, output int64) error
	}
}

// MonthUse is what a company used of this month's AI limit.
type MonthUse struct {
	UsedTokens  int64
	LimitTokens int64 // 0 when the limit is an amount of money
	UsedPercent int64
}

// Module is the assistant module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps) *Module {
	svc := &Service{pool: pool, deps: deps}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Forget deletes what the AI said about records deleted for good (subjectType
// candidate, application or job), in the caller's transaction. The tokens
// they took stay counted (aiusage's ledger).
func (m *Module) Forget(ctx context.Context, tx pgx.Tx, subjectType string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return store.New(tx).ForgetAnswers(ctx, store.ForgetAnswersParams{SubjectType: subjectType, Ids: ids})
}

// JobHandlers returns the handlers for the module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindAssistantAnswered: m.service.answered, api.JobKindAssistantEmailed: m.service.emailed}
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
