// Package interviews schedules interviews for an application and collects
// each interviewer's feedback.
//
// Scheduling moves the application to the interview stage when it is at an
// earlier one, in the same transaction. An interviewer booked at the same time
// is a clash (refused unless asked to ignore it). Interviewers see each
// other's feedback only after giving their own, so the first opinion does not
// steer the rest; those who schedule see all of it.
//
// Layout (the same in every module):
//
//	interviews.go  the module's public API and what it needs from other modules
//	domain.go      the rules, as pure functions on plain values (unit tested)
//	calendar.go    the interview as an .ics calendar file
//	reminders.go   the interviewers' reminder email the day before (a job)
//	service.go     use cases: one transaction each
//	http.go        HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/interviews.sql); private to this module
package interviews

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Application is what this module needs to know about an application.
type Application struct {
	ID, Stage                  string
	JobID, JobTitle            string
	CandidateID, CandidateName string
}

// Applications reads applications and moves them to the interview stage (pipeline module).
type Applications interface {
	Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	ReachInterview(ctx context.Context, tx pgx.Tx, id string) error
}

// Users checks interviewers and names them (identity module).
type Users interface {
	ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error)
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Message is an email to some of the company's users.
type Message struct {
	Subject, Body string
	ApplicationID string
}

// Mailer emails the company's users (identity and emails modules), in the
// caller's transaction, and says how many it emailed (inactive users are
// left out).
type Mailer interface {
	EmailUsers(ctx context.Context, tx pgx.Tx, tenantID string, userIDs []string, m Message) (int, error)
}

// Settings are the company's: its time zone (for times in reminders) and
// where people reach the app (for links).
type Settings struct {
	TimeZone  *time.Location
	PublicURL string
}

// Module is the interviews module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, applications Applications, users Users, mailer Mailer, settings Settings) *Module {
	svc := &Service{pool: pool, applications: applications, users: users, mailer: mailer, settings: settings}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindInterviewRemind: m.service.remind}
}

// Brief returns one interview for other modules, in the caller's transaction.
func (m *Module) Brief(ctx context.Context, tx pgx.Tx, id string) (Brief, error) {
	return m.service.BriefOf(ctx, tx, id)
}

// BusyFor returns when a user is in an interview between from and to, in
// the caller's transaction (booking links leave these times out).
func (m *Module) BusyFor(ctx context.Context, tx pgx.Tx, userID string, from, to time.Time) ([]ical.Interval, error) {
	rows, err := store.New(tx).BusyFor(ctx, store.BusyForParams{UserID: userID, FromTime: from, ToTime: to})
	out := make([]ical.Interval, len(rows))
	for i, r := range rows {
		out[i] = ical.Interval{Start: r.StartsAt, End: r.EndsAt}
	}
	return out, err
}
