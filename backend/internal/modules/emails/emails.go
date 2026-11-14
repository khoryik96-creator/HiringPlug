// Package emails sends email from the app: templates with placeholders filled
// from the candidate, job, interview and sender; messages queued in the same
// transaction as their record and sent by the Go worker through the
// company's mail server; and the record of what was sent, kept with the
// candidate (a merge moves it).
//
// Mail goes out from the configured address with the sender's name and
// "(company)" as the display name, and replies go to the person who sent it.
// Salary fields fill only for senders who may see salaries; an email that
// shows salary is hidden from roles that may not (unless the company chose
// otherwise in Settings).
//
// Layout (the same in every module):
//
//	emails.go     the module's public API and what it needs from other modules
//	domain.go     the rules, as pure functions on plain values (unit tested)
//	values.go     placeholder values from the candidate, job and interview
//	service.go    use cases: one transaction each
//	jobs.go       the email.send job: delivery through the mail server
//	http.go       HTTP handlers: contract types <-> service calls
//	internal/store  generated SQL (db/queries/emails.sql); private to this module
package emails

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// Candidate is what this module needs to know about a candidate.
type Candidate struct {
	Version, CompensationVersion  int
	FullName, FirstName, LastName string
	Email                         *string
	Details                       *Details // for fields; nil when only a brief was read
}

// Details are a candidate's values for fields, written as an email shows them
// (FormatSalary, FormatNotice, ...); nil when not known.
type Details struct {
	Nationality, Residence, CurrentTitle, CurrentCompany     *string
	NoticePeriod, WorkAuthorization                          *string
	SalaryNotes, CurrentBreakdown, ExpectedNotes, Industries *string
	CurrentSalary, ExpectedSalary                            *string // nil also when the sender may not see salaries
}

// Application is what this module needs to know about an application.
type Application struct {
	CandidateID   string
	JobID         string
	JobTitle      string
	CompanyName   string
	ClientEmail   *string // the job's hiring contact
	ClientContact string  // their name
}

// CVFile is a candidate's CV as an attachment.
type CVFile struct {
	CandidateID, FileName, ContentType string
	Data                               []byte
}

// Interview is what this module needs to know about an interview.
type Interview struct {
	ApplicationID string
	Title         string
	StartsAt      time.Time
	Minutes       int
	Mode          string // in_person, video, phone
	Location      *string
	Instructions  *string
}

// Deps is what the module needs from other modules, each read in the caller's
// transaction.
type Deps struct {
	Candidates interface {
		Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Candidate, error)
		// Detailed is a candidate with Details, as p may see them (salary
		// only with permission); NOT_FOUND when missing.
		Detailed(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Candidate, error)
	}
	// Documents gives the candidate's CVs to attach (the user's transaction:
	// only those they may see).
	Documents interface {
		CVFile(ctx context.Context, tx pgx.Tx, documentID string, asPDF bool) (CVFile, error)
	}
	// Policy is the company's choice on who sees emails that show salary
	// (settings module): "salary_roles" or "everyone".
	Policy interface {
		EmailSalaryVisibility(ctx context.Context, tx pgx.Tx) (string, error)
	}
	Applications interface {
		Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Application, error)
	}
	Interviews interface {
		Brief(ctx context.Context, tx pgx.Tx, id string) (Interview, error) // NOT_FOUND when missing
	}
	Tenants interface {
		TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error)
	}
	// Contacts gives a client's contact, to write to from their page
	// (contacts and companies modules); NOT_FOUND when missing.
	Contacts interface {
		Contact(ctx context.Context, tx pgx.Tx, id string) (Contact, error)
	}
	// Undeliverable says which addresses would not reach their contact
	// (contacts module: they left, or the mailbox does not exist), with why
	// in words; automated emails to them are not sent. Nil: none.
	Undeliverable interface {
		Undeliverable(ctx context.Context, tx pgx.Tx, addresses []string) (map[string]string, error)
	}
	// Consent says whether a candidate asked not to be contacted (privacy
	// module); emails to them are refused. Nil: nobody is.
	Consent interface {
		DoNotContact(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
	}
}

// Contact is one of a client's contacts, written to from their page.
type Contact struct {
	FirstName, LastName, FullName string
	CompanyName                   string
	Email                         *string
}

// Module is the emails module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. sender delivers mail (mail.Unconfigured when no mail
// server is set up: every email then fails with that reason); files keeps
// banners and attachments.
func New(pool *pgxpool.Pool, deps Deps, sender mail.Sender, files *blob.Store) *Module {
	svc := &Service{pool: pool, deps: deps, sender: sender, files: files}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindEmailSend: m.service.deliver}
}

// MoveCandidate moves a merged duplicate's emails to the candidate kept, in
// the merge's transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	if _, err := q.MoveEmailsToCandidate(ctx, store.MoveEmailsToCandidateParams{FromID: &fromID, ToID: &toID}); err != nil {
		return err
	}
	if err := q.DropShortlistDuplicates(ctx, store.DropShortlistDuplicatesParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	return q.MoveShortlistsToCandidate(ctx, store.MoveShortlistsToCandidateParams{FromID: fromID, ToID: toID})
}

// Notice is an email the app sends by itself (an interview reminder): from
// HiringPlug, with no one in particular to reply to.
type Notice struct {
	To            []string
	Subject, Body string
	ApplicationID *string
	CandidateID   *string // shown with the candidate's emails
	// Calendar is an iCalendar invitation (METHOD:REQUEST or CANCEL), sent
	// so that Gmail, Outlook and Apple Mail offer it for the calendar.
	Calendar string
	// Files go with it (an invoice as a PDF).
	Files []NoticeFile
	// ShowsFees: it shows a fee (an invoice), so only roles that may see
	// fees (placements.fees:read) see it, and its subject stays out of the
	// audit log.
	ShowsFees bool
	// SenderName is who it is from ("HiringPlug" when empty).
	SenderName string
}

// NoticeFile is a file attached to a notice. Key, when given, names it in
// storage for good (an invoice's number): a retry after a rollback writes
// the same file again instead of leaving one behind.
type NoticeFile struct {
	Name, ContentType, Key string
	Data                   []byte
}

// Notify records a notice and queues it for sending, in the caller's
// transaction (a job's, scoped to tenantID).
func (m *Module) Notify(ctx context.Context, tx pgx.Tx, tenantID string, n Notice) error {
	return m.service.notify(ctx, tx, tenantID, n)
}
