// Package shortlists is client shortlist links. A recruiter picks some of a
// job's candidates, each with one of their formatted CVs (blind, or with the
// name), and gets a link for the client, who needs no account: the public
// page shows only those candidates and CVs (never salary, email or phone),
// and the client approves or rejects each one with an optional comment.
// The answer moves the application (approved: interview; rejected: closed as
// client rejected, the comment as the note) and is on the job's and the
// candidate's activity.
//
// Only a hash of each link's secret token is kept. Links expire and can be
// revoked; a candidate who withdraws consent drops off every link.
//
// Layout (the same in every module):
//
//	shortlists.go   the module's public API and what it needs from other modules
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases for signed-in users: one transaction each
//	public.go       the client's page (no sign-in)
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/shortlists.sql); private to this module
package shortlists

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/shortlists/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// Job is the job a link is about (jobs and contacts modules).
type Job struct {
	Title        string
	ContactName  string
	ContactEmail *string // the hiring contact's; nil when there is none
}

// Application is a candidate on a job (pipeline module).
type Application struct {
	JobID, CandidateID, CandidateName, Stage string
}

// ClientCV is a CV that may go to a client (cvformats module).
type ClientCV struct {
	Blind     bool
	Reference string // "Candidate C-4F2K9Q", for a blind CV
}

// CVFile is a CV's file as the client downloads it.
type CVFile struct {
	FileName string // only its extension is used
	Data     []byte
}

// Mail is the email with the link to the hiring contact.
type Mail struct {
	To, Subject, Body string
	From              string // the recruiter: from their mailbox if connected
	FromName          string
}

// Deps are what this module needs from others, each in the caller's transaction.
type Deps struct {
	Jobs interface {
		Job(ctx context.Context, tx pgx.Tx, id string) (Job, error) // NOT_FOUND when missing
	}
	Applications interface {
		Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
		// Approved moves an application the client approved (to interview),
		// Rejected one they rejected; each returns the stage it is at
		// afterwards ("" when it is gone).
		Approved(ctx context.Context, tx pgx.Tx, id string) (string, error)
		Rejected(ctx context.Context, tx pgx.Tx, id, comment string) (string, error)
	}
	Candidates interface {
		Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	CVs interface {
		// ForClient checks a candidate's document may go to a client; reason
		// says why not, in words, when it may not.
		ForClient(ctx context.Context, tx pgx.Tx, p authz.Principal, candidateID, documentID string) (ClientCV, string, error)
		// File is the CV's PDF when it has one, else its file.
		File(ctx context.Context, tx pgx.Tx, documentID string) (CVFile, error)
	}
	// Consent says whether a candidate withdrew consent (privacy module).
	Consent interface {
		DoNotContact(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
	}
	Users interface {
		UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	// Mailer queues the email to the hiring contact (emails module).
	Mailer interface {
		Send(ctx context.Context, tx pgx.Tx, tenantID string, m Mail) error
	}
}

// Module is the shortlists module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. publicURL is where people reach the web app (the
// links' address).
func New(pool *pgxpool.Pool, deps Deps, publicURL string) *Module {
	svc := &Service{pool: pool, deps: deps, publicURL: publicURL, now: time.Now,
		limiter: ratelimit.New(time.Now, ratelimit.Limit{Window: time.Hour, Max: 120})}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// MoveCandidate moves a merged duplicate's places on links to the candidate
// kept, in the merge's transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveShortlistCandidates(ctx, store.MoveShortlistCandidatesParams{FromID: fromID, ToID: toID})
}
