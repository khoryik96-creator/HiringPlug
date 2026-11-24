// Package aiusage keeps one ledger of every AI use (which feature used the
// AI, for whom, how many input and output tokens, and when), each company's
// monthly AI limit and prices, and the one allowance check every AI feature
// uses. The ledger is append-only: a use stays counted when the record it
// was for is deleted (Bin purge, a JD file deleted, answers forgotten).
//
// Layout (the same in every module):
//
//	aiusage.go      the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases: one transaction each, rules + storage + audit
//	report.go       the month-by-month report
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/aiusage.sql); private to this module
package aiusage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config is how the server is set up.
type Config struct {
	// DefaultTokens is the monthly limit of a company that has set none
	// (HP_AI_MONTHLY_TOKENS).
	DefaultTokens int64
	// TimeZone is the agency's (HP_TIME_ZONE): a month is a calendar month there.
	TimeZone *time.Location
}

// Deps is what the module reads from other modules, in its transaction.
type Deps struct {
	// Users names the people in the report (identity module).
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
}

// Module is the aiusage module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps, config Config) *Module {
	if config.TimeZone == nil {
		config.TimeZone = time.UTC
	}
	svc := &Service{pool: pool, deps: deps, config: config, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Use is one AI run's tokens, recorded by the module that ran it.
//
// The idempotency key is (company, Feature, SourceID, RunKey): recording the
// same key again changes nothing, so a result delivered twice counts once.
//   - SourceID is the record the use belongs to, e.g. the assistant answer's,
//     fit score's, document's or JD file's ID.
//   - RunKey tells one AI run of that record from another, so a new run of
//     the same record (a CV read again, a fit score refreshed, a JD file read
//     again) counts. It must stay the same when the same result is delivered
//     twice: something the module fixed when it queued the run (e.g. the
//     request job's ID carried in the result, or the record's run number),
//     not the result job's own ID. Leave it empty for records the AI handles
//     only once (an answer, an email draft, a search).
//
// Modules still record only when their own row moves from queued to done;
// the key is the second guard.
type Use struct {
	Feature  string  // one of Features
	UserID   *string // who asked; nil when the system ran it on its own
	SourceID string
	RunKey   string
	Input    int64 // input tokens (what was sent to the AI)
	Output   int64 // output tokens (what the AI wrote)
}

// Record adds u to the ledger in the caller's transaction (any module's, a
// user's or the system's): counts are kept within 0..MaxTokens, the insert
// runs as the system for the company, and a run already recorded, or one
// that used no tokens, adds nothing. When this month's use first reaches the
// warning level or the whole limit, the level is stored (Alerts) and audited.
func (m *Module) Record(ctx context.Context, tx pgx.Tx, tenantID string, u Use) error {
	_, err := m.service.record(ctx, tx, tenantID, u)
	return err
}

// Remaining is this month's allowance left, in tokens, after what was spent
// and the holds of AI work still in progress (jobqueue.PendingAI, which also
// locks admissions for the company until the transaction ends). For a money
// limit, it is the money left in tokens at the higher price. Zero or less:
// the limit is reached and no more AI work may start this month.
func (m *Module) Remaining(ctx context.Context, tx pgx.Tx, tenantID string) (int64, error) {
	return m.service.remaining(ctx, tx, tenantID)
}

// MonthStart is when now's month began in the agency's time zone: the month
// the allowance counts.
func (m *Module) MonthStart(now time.Time) time.Time {
	return MonthStart(now, m.service.config.TimeZone)
}

// Status is how much of this month's limit a company used (work still in
// progress aside).
type Status struct {
	Kind        string // KindTokens or KindMoney
	UsedTokens  int64  // input and output together, this month
	LimitTokens int64  // the token limit (KindTokens only)
	UsedPercent int64  // can pass 100
	WarnPercent int
	Warn        bool // the warning level is reached, not the whole limit
	Reached     bool // the whole limit is used
	MonthStart  time.Time
	ResetsAt    time.Time
}

// Status reads the company's use of this month's limit, in the caller's
// transaction (as the system, so a user's visibility never hides use).
func (m *Module) Status(ctx context.Context, tx pgx.Tx, tenantID string) (Status, error) {
	return m.service.status(ctx, tx, tenantID)
}

// Alerts is the levels now's month reached so far (the warning percent,
// LimitLevel), lowest first, read in the caller's transaction (the
// notifications bell, as the user).
func (m *Module) Alerts(ctx context.Context, tx pgx.Tx, now time.Time) ([]Alert, error) {
	return m.service.alerts(ctx, tx, now)
}
