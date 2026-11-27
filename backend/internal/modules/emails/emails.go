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

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"

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
	Phone                         *string  // for a WhatsApp chat; nil when only a brief was read
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
	Salary                             bool // a formatted CV made with the salary
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
	// PhoneRegion is the country of numbers saved without a country code
	// (HP_DEFAULT_PHONE_REGION), for WhatsApp chat links.
	PhoneRegion string
	Users       interface {
		PrincipalFor(context.Context, pgx.Tx, string) (authz.Principal, bool, error)
	}
	Candidates interface {
		Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Candidate, error)
		IDsByEmails(ctx context.Context, tx pgx.Tx, addresses []string) ([]string, error)
		// Detailed is a candidate with Details, as p may see them (salary
		// only with permission); NOT_FOUND when missing.
		Detailed(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Candidate, error)
	}
	// Documents gives the candidate's CVs to attach (the user's transaction:
	// only those they may see).
	Documents interface {
		CVFile(ctx context.Context, tx pgx.Tx, documentID string, asPDF bool) (CVFile, error)
		// MaxUploadBytes is the largest file a candidate's CV & files take.
		MaxUploadBytes() int64
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
		// Touch notes that someone wrote to the contact (and so their company).
		Touch(ctx context.Context, tx pgx.Tx, id string) error
	}
	// Undeliverable says which addresses would not reach their contact
	// (contacts module: they left, or the mailbox does not exist), with why
	// in words; automated emails to them are not sent. Nil: none.
	Undeliverable interface {
		Undeliverable(ctx context.Context, tx pgx.Tx, addresses []string) (map[string]string, error)
	}
	// Bounces is told of an address a real email bounced from (contacts
	// module: their mailbox is marked as gone). Nil: nobody is.
	Bounces interface {
		Bounced(ctx context.Context, tx pgx.Tx, address string) error
	}
	// Consent says whether a candidate asked not to be contacted (privacy
	// module); emails to them are refused. Nil: nobody is.
	Consent interface {
		DoNotContact(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
	}
	Invoices interface {
		MayDeliver(ctx context.Context, tx pgx.Tx, emailID string) (bool, error)
		Delivered(ctx context.Context, tx pgx.Tx, emailID string, failed bool) error
	}
	// SignIn says whether the company signs in with Microsoft (identity
	// module): its Microsoft app is then for owners only to change. Nil: it
	// never does.
	SignIn interface {
		MicrosoftSignInOn(ctx context.Context, tx pgx.Tx) (bool, error)
	}
	InterviewReminders interface {
		ReminderBody(context.Context, pgx.Tx, string, api.InterviewReminderDelivery, []string) (string, error)
		ReminderSent(context.Context, pgx.Tx, api.InterviewReminderDelivery) error
	}
}

// Contact is one of a client's contacts, written to from their page.
type Contact struct {
	FirstName, LastName, FullName string
	CompanyName                   string
	Email                         *string
	Phone, Mobile                 *string
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
	return jobqueue.Handlers{
		api.JobKindEmailBounceScan: m.service.scanBounces, api.JobKindEmailOutlookSync: m.service.syncOutlook,
	}
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
// HyPlug, with no one in particular to reply to.
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
	// AccountNotice: about someone's account (an invitation, a password
	// reset, a link to connect Microsoft, a Microsoft account linked). Sent
	// from the server's mail, by nobody: never from the mailbox of whoever
	// caused it (a copy in their Sent Items would hand them the link), and in
	// no one's sent mail here either. The one exception is NotifyAccountFor
	// (an invitation, whose link its sender is shown anyway).
	AccountNotice bool
	// SenderName is who it is from ("HyPlug" when empty).
	SenderName string
	// SentBy, when given, is the user it is sent for: from their connected
	// mailbox if they have one (else the server's), replies going to them.
	SentBy            *string
	DeliverAt         time.Time
	InterviewReminder *api.InterviewReminderDelivery
}

// NoticeFile is a file attached to a notice. Key, when given, names it in
// storage for good (an invoice's number): a retry after a rollback writes
// the same file again instead of leaving one behind.
type NoticeFile struct {
	Name, ContentType, Key string
	Data                   []byte
}

// How an account notice from NotifyAccountFor goes.
const (
	ViaServer    = "server"     // the server's mail (HP_SMTP_*)
	ViaMailbox   = "mailbox"    // the user's own connected mailbox: the server's mail is not set up
	ViaTestInbox = "test_inbox" // the server's test inbox (Mailpit) holds it: it never arrives
	ViaNone      = "none"       // nothing can send it: no server mail, no connected mailbox
)

// NotifyAccountFor queues an account notice that userID sends and may see
// in full (an invitation: its link is shown to them too), in the caller's
// transaction. It goes from the server's mail when that is set up, else from
// userID's own connected mailbox when they have one that sends. It returns
// how it goes (Via...).
func (m *Module) NotifyAccountFor(ctx context.Context, tx pgx.Tx, tenantID, userID string, n Notice) (via, emailID string, err error) {
	n.AccountNotice = true
	via = ViaServer
	switch mode := m.service.serverMode(); {
	case mode == "live":
	case m.service.mailboxSends(ctx, tx, userID):
		n.SentBy, via = &userID, ViaMailbox
	case mode == "test":
		via = ViaTestInbox
	default:
		via = ViaNone // queued all the same: it fails, and shows so, if no mail server is set up
	}
	emailID, err = m.QueueNotice(ctx, tx, tenantID, n)
	return via, emailID, err
}

// EmailState is how an email went: queued, sent or failed (with why), and when.
type EmailState struct {
	Status string
	Error  *string
	At     time.Time
}

// EmailStates returns how some emails went, by id, in the caller's
// transaction (the Team page: invitations).
func (m *Module) EmailStates(ctx context.Context, tx pgx.Tx, ids []string) (map[string]EmailState, error) {
	out := map[string]EmailState{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := store.New(tx).EmailStates(ctx, ids)
	for _, r := range rows {
		out[r.ID] = EmailState{Status: r.Status, Error: r.Error, At: r.At}
	}
	return out, err
}

// Notify records a notice and queues it for sending, in the caller's
// transaction (a job's, scoped to tenantID).
func (m *Module) Notify(ctx context.Context, tx pgx.Tx, tenantID string, n Notice) error {
	_, err := m.QueueNotice(ctx, tx, tenantID, n)
	return err
}

func (m *Module) QueueNotice(ctx context.Context, tx pgx.Tx, tenantID string, n Notice) (string, error) {
	return m.service.notify(ctx, tx, tenantID, n)
}

// InvoicePDF returns the actual attachment on the fee-protected email.
func (m *Module) InvoicePDF(ctx context.Context, tx pgx.Tx, emailID string) ([]byte, error) {
	files, err := store.New(tx).EmailAttachments(ctx, []string{emailID})
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.ContentType == "application/pdf" {
			return m.service.read(file.StorageKey)
		}
	}
	return nil, apperr.NotFound("invoice PDF")
}

// MoveContact moves the emails to contact fromID, and everyone's Outlook
// threads linked to them (those the merger may not see too), to contact toID, inside the transaction merging two
// duplicate contacts.
func (m *Module) MoveContact(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveContactEmails(ctx, store.MoveContactEmailsParams{FromID: fromID, ToID: toID})
}
