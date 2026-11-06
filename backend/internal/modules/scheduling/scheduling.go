// Package scheduling is a small Calendly of our own: each user's hours for
// calls (and their own calendar's busy times, read from the private address
// Google Calendar or Outlook gives), booking links sent to candidates, a
// public page to pick a time, and calendar invitations that block it.
//
// Layout (the same in every module):
//
//	scheduling.go   the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases for signed-in users: one transaction each
//	public.go       the public booking page (no sign-in)
//	invites.go      the calendar invitations
//	jobs.go         reading a user's calendar address (core queue)
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/scheduling.sql); private to this module
package scheduling

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// Person is someone with a name and maybe an email address.
type Person struct {
	Name  string
	Email *string
}

// Mail is an email with a calendar invitation.
type Mail struct {
	To                         string
	Subject, Body, Calendar    string
	CandidateID, ApplicationID *string
}

// Deps are what this module needs from others.
type Deps struct {
	// Candidates gives a candidate's name and email, as p may see them (a
	// candidate p may not see is "not found").
	Candidates interface {
		Person(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Person, error)
	}
	// Applications gives an application's candidate ("" when there is none).
	Applications interface {
		CandidateOf(ctx context.Context, tx pgx.Tx, applicationID string) (string, error)
	}
	// Users gives a user's name and email (nil when not active), and the
	// company's name.
	Users interface {
		Person(ctx context.Context, tx pgx.Tx, userID string) (*Person, error)
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	// Interviews gives the interviews a user is on: busy times.
	Interviews interface {
		BusyFor(ctx context.Context, tx pgx.Tx, userID string, from, to time.Time) ([]ical.Interval, error)
	}
	// Mailer sends an email with an invitation (queued in the transaction).
	Mailer interface {
		Send(ctx context.Context, tx pgx.Tx, tenantID string, m Mail) error
	}
}

// Config is how the server is set up.
type Config struct {
	PublicURL            string         // where booking pages are, e.g. https://ats.example.com
	OrganizerEmail       string         // invitations come from this address (HP_SMTP_FROM)
	DefaultTimeZone      *time.Location // for users who have not chosen one
	AllowPrivateNetworks bool           // calendar addresses on private networks (tests only)
}

// Module is the scheduling module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps, settings Config) *Module {
	if settings.DefaultTimeZone == nil {
		settings.DefaultTimeZone = time.UTC
	}
	if settings.OrganizerEmail == "" {
		settings.OrganizerEmail = "no-reply@hiringplug.local"
	}
	svc := &Service{pool: pool, deps: deps, settings: settings, now: time.Now,
		limiter: ratelimit.New(time.Now, ratelimit.Limit{Window: time.Hour, Max: 10})}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindSchedulingRefreshCalendar: m.service.refreshCalendar}
}
