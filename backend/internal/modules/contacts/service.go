package contacts

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const (
	emailUniqueIndex = "contact_tenant_email_uq"
	lookupTimeout    = 3 * time.Second
)

// Service holds the contacts use cases.
type Service struct {
	pool      *pgxpool.Pool
	companies Companies
	mail      MailServers
}

// Create adds a contact. The email is checked before the transaction starts
// (it needs the network), so no transaction waits on a slow mail server.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Details) (Contact, error) {
	d, errs := Normalize(in)
	if err := errs.Err(); err != nil {
		return Contact{}, err
	}
	check := s.checkEmail(ctx, d.Email)
	var out Contact
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := s.checkCompany(ctx, tx, d.CompanyID); err != nil {
			return err
		}
		if err := checkEmailFree(ctx, q, d.Email, ""); err != nil {
			return err
		}
		params := store.InsertContactParams{
			CompanyID: d.CompanyID, FirstName: d.FirstName, LastName: d.LastName, Title: d.Title, Email: d.Email,
			EmailCheckReasons: []string{}, Phone: d.Phone, Mobile: d.Mobile, LinkedinUrl: d.LinkedinURL,
			Status: d.Status, Notes: d.Notes,
		}
		if check != nil {
			params.EmailCheckStatus, params.EmailCheckReasons, params.EmailCheckedAt = &check.Status, check.Reasons, &check.CheckedAt
		}
		row, err := q.InsertContact(ctx, params)
		if database.IsUniqueViolation(err, emailUniqueIndex) {
			return duplicateEmail("")
		}
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			Action: "contact.created", EntityType: "contact", EntityID: row.ID,
			Changes: audit.Diff(map[string]any{}, fields(d)),
		}); err != nil {
			return err
		}
		out, err = s.withCompanyName(ctx, tx, fromRow(row))
		return err
	})
	return out, err
}

// Get returns one contact.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (Contact, error) {
	var out Contact
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := getRow(ctx, store.New(tx), id)
		if err != nil {
			return err
		}
		out, err = s.withCompanyName(ctx, tx, fromRow(row))
		return err
	})
	return out, err
}

// Update changes some fields; expectedVersion must be the version the user
// loaded. A new email is checked (before the transaction). Nothing is written
// when nothing changes.
func (s *Service) Update(ctx context.Context, p authz.Principal, id string, expectedVersion int, patch Patch) (Contact, error) {
	var check *EmailCheckResult
	if patch.Email.Set && patch.Email.Value != nil {
		if d, _ := Normalize(Details{FirstName: "x", Email: patch.Email.Value}); d.Email != nil {
			check = s.checkEmail(ctx, d.Email)
		}
	}
	var out Contact
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := getRow(ctx, q, id)
		if err != nil {
			return err
		}
		if int(row.Version) != expectedVersion {
			return apperr.StaleVersion("contact").WithDetail("currentVersion", row.Version)
		}
		current := fromRow(row)
		after, errs := Normalize(patch.Apply(current.Details))
		if err := errs.Err(); err != nil {
			return err
		}
		changes := audit.Diff(fields(current.Details), fields(after))
		if len(changes) == 0 {
			out, err = s.withCompanyName(ctx, tx, current)
			return err
		}
		if after.CompanyID != current.Details.CompanyID {
			if err := s.checkCompany(ctx, tx, after.CompanyID); err != nil {
				return err
			}
		}
		if err := checkEmailFree(ctx, q, after.Email, id); err != nil {
			return err
		}
		result := current.EmailCheck // same email: keep its result
		if !sameString(after.Email, current.Details.Email) {
			result = check
		}
		params := store.UpdateContactParams{
			ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // versions are small positive integers
			CompanyID: after.CompanyID, FirstName: after.FirstName, LastName: after.LastName, Title: after.Title,
			Email: after.Email, EmailCheckReasons: []string{}, Phone: after.Phone, Mobile: after.Mobile,
			LinkedinUrl: after.LinkedinURL, Status: after.Status, Notes: after.Notes,
		}
		if after.Email != nil && result != nil {
			params.EmailCheckStatus, params.EmailCheckReasons, params.EmailCheckedAt = &result.Status, result.Reasons, &result.CheckedAt
		}
		updated, err := q.UpdateContact(ctx, params)
		switch {
		case database.IsNoRows(err):
			return apperr.StaleVersion("contact")
		case database.IsUniqueViolation(err, emailUniqueIndex):
			return duplicateEmail("")
		case err != nil:
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "contact.updated", EntityType: "contact", EntityID: id, Changes: changes}); err != nil {
			return err
		}
		out, err = s.withCompanyName(ctx, tx, fromRow(updated))
		return err
	})
	return out, err
}

// CheckEmailAgain re-runs the email check and stores the result.
func (s *Service) CheckEmailAgain(ctx context.Context, p authz.Principal, id string) (Contact, error) {
	current, err := s.Get(ctx, p, id)
	if err != nil || current.Details.Email == nil {
		return current, err
	}
	check := s.checkEmail(ctx, current.Details.Email)
	var out Contact
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.SetEmailCheck(ctx, store.SetEmailCheckParams{
			ID: id, Email: current.Details.Email, EmailCheckStatus: &check.Status,
			EmailCheckReasons: check.Reasons, EmailCheckedAt: &check.CheckedAt,
		})
		if database.IsNoRows(err) { // the email changed meanwhile: show the current state
			row, err = getRow(ctx, q, id)
		}
		if err != nil {
			return err
		}
		out, err = s.withCompanyName(ctx, tx, fromRow(row))
		return err
	})
	return out, err
}

// Page is one page of a list, newest first.
type Page struct {
	Items      []Summary
	NextCursor *string
}

// List returns up to limit contacts, optionally matching query or one company.
// With similar, names like query (typos) instead, the closest first, one page only.
func (s *Service) List(ctx context.Context, p authz.Principal, query, companyID, cursor *string, similar bool, limit int) (Page, error) {
	var page Page
	similar = similar && query != nil
	if similar {
		cursor = nil // ordered by likeness, not by id: one page
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).ListContacts(ctx, store.ListContactsParams{
			Cursor: cursor, CompanyID: companyID, Query: query, SimilarNames: similar, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
		})
		if err != nil {
			return err
		}
		if similar && len(rows) > limit {
			rows = rows[:limit]
		}
		if len(rows) > limit {
			rows = rows[:limit]
			last := rows[len(rows)-1].ID
			page.NextCursor = &last
		}
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.CompanyID
		}
		names, err := s.companies.Names(ctx, tx, ids)
		if err != nil {
			return err
		}
		page.Items = make([]Summary, len(rows))
		for i, r := range rows {
			page.Items[i] = Summary{
				ID: r.ID, CompanyID: r.CompanyID, CompanyName: names[r.CompanyID], FirstName: r.FirstName, LastName: r.LastName,
				Status: r.Status, Title: r.Title, Email: r.Email, EmailCheckStatus: r.EmailCheckStatus,
				Phone: r.Phone, Mobile: r.Mobile, UpdatedAt: r.UpdatedAt,
			}
		}
		return nil
	})
	return page, err
}

// Exists reports whether a contact exists in the transaction's tenant.
func (s *Service) Exists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	return store.New(tx).ContactExists(ctx, id)
}

// Briefs returns id -> name and company for the contacts that exist.
func (s *Service) Briefs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Brief, error) {
	rows, err := store.New(tx).ContactBriefs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Brief, len(rows))
	for _, r := range rows {
		out[r.ID] = Brief{Name: FullName(r.FirstName, r.LastName), CompanyID: r.CompanyID, Email: r.Email}
	}
	return out, nil
}

// checkEmail runs the email check for an address (nil without one).
func (s *Service) checkEmail(ctx context.Context, email *string) *EmailCheckResult {
	if email == nil {
		return nil
	}
	server := MailServerFound
	if WellFormedEmail(*email) {
		server = s.lookupMailServer(ctx, EmailDomain(*email))
	}
	status, reasons := CheckEmail(*email, server)
	if reasons == nil {
		reasons = []string{}
	}
	return &EmailCheckResult{Status: status, Reasons: reasons, CheckedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

// lookupMailServer asks DNS where domain receives mail: its MX records, or,
// without any, its address (RFC 5321's implicit MX). A null MX means "no mail".
func (s *Service) lookupMailServer(ctx context.Context, domain string) string {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	mx, err := s.mail.LookupMX(ctx, domain)
	if err == nil && len(mx) > 0 {
		if len(mx) == 1 && (mx[0].Host == "." || mx[0].Host == "") {
			return MailServerNone
		}
		return MailServerFound
	}
	if err != nil && !notFound(err) {
		return MailServerFailed
	}
	if _, err := s.mail.LookupHost(ctx, domain); err != nil {
		if notFound(err) && s.dnsWorks(ctx) {
			return MailServerNone
		}
		return MailServerFailed
	}
	return MailServerFound
}

// controlDomain surely receives mail. If even it "does not exist", DNS itself
// is broken (offline, no resolver): say "unknown" rather than mark every
// address as bouncing.
const controlDomain = "gmail.com"

func (s *Service) dnsWorks(ctx context.Context) bool {
	mx, err := s.mail.LookupMX(ctx, controlDomain)
	return err == nil && len(mx) > 0
}

func notFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func (s *Service) checkCompany(ctx context.Context, tx pgx.Tx, companyID string) error {
	ok, err := s.companies.Exists(ctx, tx, companyID)
	if err != nil {
		return err
	}
	if !ok {
		var errs apperr.FieldErrors
		errs.Add("companyId", "no such company")
		return errs.Err()
	}
	return nil
}

func (s *Service) withCompanyName(ctx context.Context, tx pgx.Tx, c Contact) (Contact, error) {
	names, err := s.companies.Names(ctx, tx, []string{c.Details.CompanyID})
	c.CompanyName = names[c.Details.CompanyID]
	return c, err
}

func getRow(ctx context.Context, q *store.Queries, id string) (store.Contact, error) {
	row, err := q.GetContact(ctx, id)
	if database.IsNoRows(err) {
		return row, apperr.NotFound("contact")
	}
	return row, err
}

func checkEmailFree(ctx context.Context, q *store.Queries, email *string, selfID string) error {
	if email == nil {
		return nil
	}
	existing, err := q.FindContactIDByEmail(ctx, email)
	if database.IsNoRows(err) || (err == nil && existing == selfID) {
		return nil
	}
	if err != nil {
		return err
	}
	return duplicateEmail(existing)
}

func duplicateEmail(existingID string) error {
	e := apperr.New(api.ErrorCodeCONTACTDUPLICATEEMAIL, "Another contact already has this email.")
	if existingID != "" {
		e = e.WithDetail("existingContactId", existingID)
	}
	return e
}

func sameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func fromRow(r store.Contact) Contact {
	c := Contact{
		ID: r.ID, Version: int(r.Version), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Details: Details{
			CompanyID: r.CompanyID, FirstName: r.FirstName, LastName: r.LastName, Title: r.Title, Email: r.Email,
			Phone: r.Phone, Mobile: r.Mobile, LinkedinURL: r.LinkedinUrl, Status: r.Status, Notes: r.Notes,
		},
	}
	if r.EmailCheckStatus != nil && r.EmailCheckedAt != nil {
		c.EmailCheck = &EmailCheckResult{Status: *r.EmailCheckStatus, Reasons: r.EmailCheckReasons, CheckedAt: *r.EmailCheckedAt}
	}
	return c
}

// fields lists the values the audit log compares.
func fields(d Details) map[string]any {
	return map[string]any{
		"companyId": d.CompanyID, "firstName": d.FirstName, "lastName": d.LastName, "title": d.Title,
		"email": d.Email, "phone": d.Phone, "mobile": d.Mobile, "linkedinUrl": d.LinkedinURL,
		"status": d.Status, "notes": d.Notes,
	}
}
