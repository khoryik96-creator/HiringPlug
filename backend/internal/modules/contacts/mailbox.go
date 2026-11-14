package contacts

// Mailbox checks: does a contact's mailbox exist? Asked of their mail server
// in the background (platform/mailcheck), one contact or a whole company at
// a time. A clear "does not exist" shows as invalid (an email would bounce);
// anything inconclusive stays unknown, never invalid.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mailcheck"
)

// MailboxCheckStatus "checking": asked, not answered yet.
const MailboxChecking = "checking"

// controlLifetime is how long a check's made-up mailbox is reused by rechecks.
const controlLifetime = 4 * time.Hour

// MailboxCheck is the latest check of a contact's email.
type MailboxCheck struct {
	Email, Status        string
	Reason, Provider     *string
	Attempts             int
	RecheckAt, CheckedAt *time.Time
	RequestedAt          time.Time
}

func mailboxFrom(r store.ContactMailboxCheck) MailboxCheck {
	return MailboxCheck{Email: r.Email, Status: r.Status, Reason: r.Reason, Provider: r.Provider, Attempts: int(r.Attempts),
		RecheckAt: r.RecheckAt, CheckedAt: r.CheckedAt, RequestedAt: r.RequestedAt}
}

// start marks a contact's email as being checked and queues the check.
func start(ctx context.Context, tx pgx.Tx, tenantID, contactID, email string) (store.ContactMailboxCheck, error) {
	row, err := store.New(tx).StartMailboxCheck(ctx, store.StartMailboxCheckParams{
		ContactID: contactID, Email: email, ControlID: mailcheck.NewControlID(), ControlExpiresAt: time.Now().Add(controlLifetime),
	})
	if err != nil {
		return row, err
	}
	_, err = jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindContactCheckMailbox, api.ContactCheckMailboxJob{ContactId: contactID, Email: email})
	return row, err
}

// CheckMailbox starts a check of a contact's email.
func (s *Service) CheckMailbox(ctx context.Context, p authz.Principal, contactID string) (MailboxCheck, error) {
	var out MailboxCheck
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getRow(ctx, store.New(tx), contactID)
		if err != nil {
			return err
		}
		if row.Email == nil {
			var errs apperr.FieldErrors
			errs.Add("email", "add an email address first")
			return errs.Err()
		}
		check, err := start(ctx, tx, p.TenantID, contactID, *row.Email)
		if err != nil {
			return err
		}
		out = mailboxFrom(check)
		return audit.Record(ctx, tx, audit.Entry{Action: "contact.mailbox_check_started", EntityType: "contact", EntityID: contactID})
	})
	return out, err
}

// CheckCompanyMailboxes starts a check of each of a company's contacts'
// emails, except those being checked already.
func (s *Service) CheckCompanyMailboxes(ctx context.Context, p authz.Principal, companyID string) (started, skipped int, err error) {
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := s.checkCompany(ctx, tx, companyID); err != nil {
			return err
		}
		rows, err := store.New(tx).CompanyContactsWithEmail(ctx, companyID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.MailboxStatus != nil && *r.MailboxStatus == MailboxChecking {
				skipped++
				continue
			}
			if _, err := start(ctx, tx, p.TenantID, r.ID, r.Email); err != nil {
				return err
			}
			started++
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "contact.mailbox_checks_started", EntityType: "company", EntityID: companyID,
			Changes: map[string]any{"started": started}})
	})
	return started, skipped, err
}

// runMailboxCheck handles contact.check_mailbox: asks the mail server (up to
// 25 seconds) and saves the answer. An inconclusive answer that may change
// (a busy or greylisting server) is checked again once, two minutes later,
// comparing the domain's servers, with the same made-up mailbox.
func (s *Service) runMailboxCheck(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var j api.ContactCheckMailboxJob
	if err := job.Decode(&j); err != nil {
		return err
	}
	q := store.New(tx)
	check, err := q.GetMailboxCheck(ctx, j.ContactId)
	if database.IsNoRows(err) {
		return nil // the contact was deleted
	}
	if err != nil {
		return err
	}
	due := check.Status == MailboxChecking || (j.Deep && check.RecheckAt != nil)
	contact, err := q.GetContact(ctx, j.ContactId)
	if err != nil && !database.IsNoRows(err) {
		return err
	}
	if !due || check.Email != j.Email || err != nil || contact.Email == nil || *contact.Email != j.Email {
		return nil // superseded: a newer check, or the email changed
	}
	mode := mailcheck.Full
	if j.Deep {
		mode = mailcheck.Deep
	}
	r, err := s.mailboxes.Check(ctx, j.Email, mode, check.ControlID)
	if err != nil {
		return err // stopped (shutting down): the job runs again
	}
	params := store.SaveMailboxCheckParams{
		ContactID: j.ContactId, Email: j.Email, Status: r.Status, Reason: ptr(clip(r.Reason, 500)),
		Outcome: ptr(r.Outcome), Provider: ptr(r.Provider), Attempts: int32(min(r.Attempts, 10)), //nolint:gosec // at most 4
		Rechecks: check.Rechecks,
	}
	if r.Provider == "Not checked" {
		params.Provider = nil
	}
	if r.Status == mailcheck.Unknown && r.Retryable && check.Rechecks < 1 {
		at := time.Now().Add(mailcheck.RetryAfter)
		params.RecheckAt, params.Rechecks = &at, check.Rechecks+1
		if _, err := jobqueue.EnqueueAt(ctx, tx, job.TenantID, api.JobKindContactCheckMailbox,
			api.ContactCheckMailboxJob{ContactId: j.ContactId, Email: j.Email, Deep: true}, at); err != nil {
			return err
		}
	}
	if _, err := q.SaveMailboxCheck(ctx, params); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "contact.mailbox_checked", EntityType: "contact", EntityID: j.ContactId,
		Changes: map[string]any{"status": r.Status, "outcome": r.Outcome}})
}

// mailboxOf returns the contact's latest check if it is of their current email.
func mailboxOf(ctx context.Context, q *store.Queries, contactID string, email *string) (*MailboxCheck, error) {
	if email == nil {
		return nil, nil //nolint:nilnil // no email, no check
	}
	row, err := q.GetMailboxCheck(ctx, contactID)
	if database.IsNoRows(err) || (err == nil && row.Email != *email) {
		return nil, nil //nolint:nilnil // not checked (or another email was)
	}
	if err != nil {
		return nil, err
	}
	m := mailboxFrom(row)
	return &m, nil
}

// CheckContactMailbox handles POST /v1/contacts/{contactId}/mailbox-check.
func (h *HTTP) CheckContactMailbox(ctx context.Context, req api.CheckContactMailboxRequestObject) (api.CheckContactMailboxResponseObject, error) {
	m, err := h.svc.CheckMailbox(ctx, authz.MustFromContext(ctx), req.ContactId)
	if err != nil {
		return nil, err
	}
	return api.CheckContactMailbox202JSONResponse(mailboxToAPI(m)), nil
}

// CheckCompanyMailboxes handles POST /v1/contacts/mailbox-checks.
func (h *HTTP) CheckCompanyMailboxes(ctx context.Context, req api.CheckCompanyMailboxesRequestObject) (api.CheckCompanyMailboxesResponseObject, error) {
	started, skipped, err := h.svc.CheckCompanyMailboxes(ctx, authz.MustFromContext(ctx), req.Body.CompanyId)
	if err != nil {
		return nil, err
	}
	return api.CheckCompanyMailboxes202JSONResponse{Started: started, Skipped: skipped}, nil
}

func mailboxToAPI(m MailboxCheck) api.ContactMailboxCheck {
	return api.ContactMailboxCheck{
		Email: m.Email, Status: api.MailboxCheckStatus(m.Status), Reason: c.OrNull(m.Reason), Provider: c.OrNull(m.Provider),
		Attempts: m.Attempts, RecheckAt: c.OrNull(m.RecheckAt), RequestedAt: m.RequestedAt, CheckedAt: c.OrNull(m.CheckedAt),
	}
}

func ptr(s string) *string { return &s }
