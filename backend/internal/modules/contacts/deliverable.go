package contacts

// Addresses that would not reach their contact: they left, or their mailbox
// does not exist. Asked by the email composer (to warn) and by automated
// emails (not sent; the recruiter gets a task).

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mailcheck"
)

// Address problems.
const (
	ProblemBounces = "bounces" // their mail server says the mailbox does not exist
	ProblemLeft    = "left"    // they have left the company
)

// AddressProblem is an address that would not reach its contact.
type AddressProblem struct {
	Email, ContactID, ContactName, CompanyName, Problem string
}

// Why says, in words, why an email to it would not reach them.
func (a AddressProblem) Why() string {
	if a.Problem == ProblemLeft {
		return a.ContactName + " (" + a.Email + ") has left " + a.CompanyName
	}
	return a.ContactName + "'s address " + a.Email + " bounces: the mailbox does not exist"
}

func (s *Service) undeliverable(ctx context.Context, tx pgx.Tx, emails []string) ([]AddressProblem, error) {
	lower := make([]string, 0, len(emails))
	for _, e := range emails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			lower = append(lower, e)
		}
	}
	if len(lower) == 0 {
		return nil, nil
	}
	rows, err := store.New(tx).UndeliverableContacts(ctx, lower)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.CompanyID
	}
	names, err := s.companies.Names(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AddressProblem, len(rows))
	for i, r := range rows {
		out[i] = AddressProblem{Email: r.Email, ContactID: r.ID, ContactName: FullName(r.FirstName, r.LastName), CompanyName: names[r.CompanyID], Problem: ProblemBounces}
		if r.Status == "left" {
			out[i].Problem = ProblemLeft // the stronger reason
		}
	}
	return out, nil
}

// Undeliverable returns the addresses among emails that would not reach
// their contact, in the caller's transaction (for other modules).
func (m *Module) Undeliverable(ctx context.Context, tx pgx.Tx, emails []string) ([]AddressProblem, error) {
	return m.service.undeliverable(ctx, tx, emails)
}

// CheckAddresses is Undeliverable for the email composer.
func (s *Service) CheckAddresses(ctx context.Context, p authz.Principal, emails []string) ([]AddressProblem, error) {
	var out []AddressProblem
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.undeliverable(ctx, tx, emails)
		return err
	})
	return out, err
}

// CheckContactAddresses handles POST /v1/contacts/address-check.
func (h *HTTP) CheckContactAddresses(ctx context.Context, req api.CheckContactAddressesRequestObject) (api.CheckContactAddressesResponseObject, error) {
	problems, err := h.svc.CheckAddresses(ctx, authz.MustFromContext(ctx), req.Body.Emails)
	if err != nil {
		return nil, err
	}
	out := api.CheckContactAddresses200JSONResponse{Items: make([]api.ContactAddressProblem, len(problems))}
	for i, a := range problems {
		out.Items[i] = api.ContactAddressProblem{Email: a.Email, ContactId: a.ContactID, ContactName: a.ContactName, CompanyName: a.CompanyName,
			Problem: api.ContactAddressProblemProblem(a.Problem)}
	}
	return out, nil
}

// Bounced marks the mailbox of the contact at address as gone: a real email
// to it bounced (in the caller's transaction, for the emails module).
func (m *Module) Bounced(ctx context.Context, tx pgx.Tx, address string) error {
	ids, err := store.New(tx).MarkMailboxBounced(ctx, store.MarkMailboxBouncedParams{
		Email: strings.ToLower(strings.TrimSpace(address)), ControlID: mailcheck.NewControlID(),
		Reason: ptr("A real email to this address bounced on " + time.Now().UTC().Format("2 Jan 2006") + ": the mailbox does not exist."),
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := audit.Record(ctx, tx, audit.Entry{Action: "contact.mailbox_bounced", EntityType: "contact", EntityID: id}); err != nil {
			return err
		}
	}
	return nil
}
