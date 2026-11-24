// Package notifications is the bell in the top bar and the morning email:
// new applicants on your jobs, your tasks due, offers waiting for your
// approval, your interviews today and tomorrow, the CVs you had formatted,
// and (for who may see the company's AI use) this month's AI use reaching
// the warning level or the limit. Nothing is copied: the
// list is worked out from the other modules' records when asked, as the
// user (so they see only what they may see). This module keeps only, per
// user, when they last looked and whether they want the morning email.
//
// Layout (the same in every module):
//
//	notifications.go  the module's public API: what internal/app and other modules may use
//	domain.go         the rules, as pure functions on plain values (unit tested)
//	service.go        use cases: one transaction each, rules + storage + audit
//	digest.go         the morning email's job
//	http.go           HTTP handlers: contract types <-> service calls; no business rules
//	internal/store    generated SQL (db/queries/notifications.sql); private to this module
package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Sources is what the notifications are made from, read from the modules
// that own the records, each in the caller's transaction (scoped to the
// user, so row-level security leaves out what they may not see).
type Sources interface {
	// OwnedJobs is the jobs the user owns.
	OwnedJobs(ctx context.Context, tx pgx.Tx, userID string) ([]string, error)
	// Applicants is who applied to these jobs after since (not added by a recruiter).
	Applicants(ctx context.Context, tx pgx.Tx, jobIDs []string, since time.Time) ([]Record, error)
	// DueTasks is the user's open tasks due on or before day.
	DueTasks(ctx context.Context, tx pgx.Tx, userID string, day time.Time) ([]Task, error)
	// OffersToApprove is the offers waiting for approval the user did not ask for.
	OffersToApprove(ctx context.Context, tx pgx.Tx, userID string) ([]Record, error)
	// Interviews is the user's scheduled interviews starting in [from, to).
	Interviews(ctx context.Context, tx pgx.Tx, userID string, from, to time.Time) ([]Interview, error)
	// FormattedCVs is the CVs the user had formatted CV Studio's way that
	// finished since then, with the candidate's name.
	FormattedCVs(ctx context.Context, tx pgx.Tx, userID string, since time.Time) ([]FormattedCV, error)
	// AILimits is the levels the company's AI use reached in now's month
	// (its warning percent, AILimitLevel).
	AILimits(ctx context.Context, tx pgx.Tx, now time.Time) ([]AILimit, error)
	// Applications names the candidate and job of applications (missing ones are left out).
	Applications(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
}

// Users is the identity module, for the morning email: the user as the app
// acts for them (false when they are no longer an active user).
type Users interface {
	PrincipalFor(ctx context.Context, tx pgx.Tx, userID string) (authz.Principal, bool, error)
}

// Mailer emails a user of the company (false: they have no active address).
type Mailer interface {
	EmailUser(ctx context.Context, tx pgx.Tx, tenantID, userID, subject, body string) (bool, error)
}

// Settings is how the module is set up.
type Settings struct {
	TimeZone  *time.Location // the agency's (HP_TIME_ZONE): "today" and 8:00 are there
	PublicURL string         // links in the morning email
}

// Module is the notifications module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, src Sources, users Users, mail Mailer, cfg Settings) *Module {
	if cfg.TimeZone == nil {
		cfg.TimeZone = time.UTC
	}
	svc := &Service{pool: pool, src: src, users: users, mail: mail, cfg: cfg, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the handlers for the module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindNotificationsDigest: m.service.sendDigest}
}

// UseClock replaces the clock (tests: a morning email at a fixed time).
func (m *Module) UseClock(now func() time.Time) { m.service.now = now }
