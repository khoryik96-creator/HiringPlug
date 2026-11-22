package contacts

// Duplicates: contacts that look like the same person (the same LinkedIn
// profile, name or phone, or a name much alike at the same company), and
// merging one into another; and moving a company's contacts when two
// companies are merged.

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/dupgroups"
)

// RecordMover moves another module's records about a contact (emails, jobs,
// invoices, notes...) when two duplicates are merged, inside the merge's
// transaction.
type RecordMover interface {
	MoveContact(ctx context.Context, tx pgx.Tx, fromID, toID string) error
}

// MoveRecordsWith sets the modules whose records follow a merge. Called by
// internal/app once every module exists.
func (m *Module) MoveRecordsWith(movers ...RecordMover) { m.service.movers = movers }

// MoveCompany moves company fromID's contacts, LinkedIn checks and HR
// searches to company toID, inside the transaction merging two companies.
func (m *Module) MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	if _, err := q.MoveCompanyContacts(ctx, store.MoveCompanyContactsParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	if err := q.MoveCompanyLinkedinChecks(ctx, store.MoveCompanyLinkedinChecksParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	return q.MoveCompanyHRScans(ctx, store.MoveCompanyHRScansParams{FromID: fromID, ToID: toID})
}

// FillFrom returns keep with each empty field taken from dup, and the names
// of the fields filled. The name, company and status stay keep's; both
// notes are kept.
func FillFrom(keep, dup Details) (Details, []string) {
	var filled []string
	fill := func(name string, k **string, d *string) {
		if *k == nil && d != nil {
			*k = d
			filled = append(filled, name)
		}
	}
	fill("title", &keep.Title, dup.Title)
	fill("email", &keep.Email, dup.Email)
	fill("phone", &keep.Phone, dup.Phone)
	fill("mobile", &keep.Mobile, dup.Mobile)
	fill("linkedinUrl", &keep.LinkedinURL, dup.LinkedinURL)
	fill("role", &keep.Role, dup.Role)
	if keep.Notes != nil && dup.Notes != nil && strings.TrimSpace(*dup.Notes) != strings.TrimSpace(*keep.Notes) {
		joined := *keep.Notes + "\n\n" + *dup.Notes
		keep.Notes, filled = &joined, append(filled, "notes")
	} else {
		fill("notes", &keep.Notes, dup.Notes)
	}
	return keep, filled
}

// Lookalike is a possible duplicate and why.
type Lookalike struct {
	ID, CompanyID, FullName string
	CompanyName             *string
	Title, Email            *string
	Reasons                 []api.ContactDuplicateReason
}

// Similar returns the possible duplicates of contact id.
func (s *Service) Similar(ctx context.Context, p authz.Principal, id string) ([]Lookalike, error) {
	out := []Lookalike{}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := getRow(ctx, q, id); err != nil {
			return err
		}
		rows, err := q.SimilarContacts(ctx, id)
		if err != nil {
			return err
		}
		for _, r := range rows {
			l := Lookalike{ID: r.ID, CompanyID: r.CompanyID, FullName: strings.TrimSpace(r.FirstName + " " + r.LastName), Title: r.Title, Email: r.Email}
			for reason, on := range map[api.ContactDuplicateReason]bool{
				api.ContactDuplicateReasonSameLinkedin: r.SameLinkedin, api.ContactDuplicateReasonSameName: r.SameName,
				api.ContactDuplicateReasonSamePhone: r.SamePhone, api.ContactDuplicateReasonSimilarName: r.SimilarName && !r.SameName, // the same name says it already
			} {
				if on {
					l.Reasons = append(l.Reasons, reason)
				}
			}
			slices.Sort(l.Reasons)
			out = append(out, l)
		}
		return s.nameCompanies(ctx, tx, out)
	})
	return out, err
}

// DuplicateGroup is contacts that look like one person, and why.
type DuplicateGroup struct {
	Reasons  []api.ContactDuplicateReason
	Contacts []Lookalike
}

// Duplicates returns every group of possible duplicates, newest first.
func (s *Service) Duplicates(ctx context.Context, p authz.Principal) ([]DuplicateGroup, error) {
	out := []DuplicateGroup{}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ContactDuplicateGroups(ctx)
		if err != nil {
			return err
		}
		groups := make([]dupgroups.Group, len(rows))
		for i, r := range rows {
			groups[i] = dupgroups.Group{Reason: r.Kind, IDs: r.Ids}
		}
		joined := dupgroups.Join(groups)
		var ids []string
		for _, g := range joined {
			ids = append(ids, g.IDs...)
		}
		found, err := q.ContactLookalikes(ctx, ids)
		if err != nil {
			return err
		}
		byID := map[string]Lookalike{}
		for _, f := range found {
			byID[f.ID] = Lookalike{ID: f.ID, CompanyID: f.CompanyID, FullName: strings.TrimSpace(f.FirstName + " " + f.LastName), Title: f.Title, Email: f.Email}
		}
		var all []Lookalike
		for _, g := range joined {
			dg := DuplicateGroup{Reasons: []api.ContactDuplicateReason{}}
			for _, r := range g.Reasons {
				dg.Reasons = append(dg.Reasons, api.ContactDuplicateReason(r))
			}
			for _, id := range g.IDs {
				if l, ok := byID[id]; ok {
					dg.Contacts = append(dg.Contacts, l)
				}
			}
			if len(dg.Contacts) > 1 {
				out = append(out, dg)
				all = append(all, dg.Contacts...)
			}
		}
		if err := s.nameCompanies(ctx, tx, all); err != nil {
			return err
		}
		i := 0
		for g := range out { // all holds the groups' contacts in order: put the names back
			for c := range out[g].Contacts {
				out[g].Contacts[c] = all[i]
				i++
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) nameCompanies(ctx context.Context, tx pgx.Tx, list []Lookalike) error {
	ids := make([]string, 0, len(list))
	for _, l := range list {
		ids = append(ids, l.CompanyID)
	}
	names, err := s.companies.Names(ctx, tx, ids)
	if err != nil {
		return err
	}
	for i := range list {
		if n, ok := names[list[i].CompanyID]; ok {
			list[i].CompanyName = &n
		}
		if list[i].Reasons == nil {
			list[i].Reasons = []api.ContactDuplicateReason{}
		}
	}
	return nil
}

// Merge moves contact dupID into keepID (their emails, jobs, invoices,
// notes, folder places and checks; their values for keepID's empty fields)
// and deletes them, in one transaction. version is keepID's version as loaded.
func (s *Service) Merge(ctx context.Context, p authz.Principal, keepID, dupID string, version int) (Contact, error) {
	if keepID == dupID {
		var errs apperr.FieldErrors
		errs.Add("duplicateId", "is the same contact")
		return Contact{}, errs.Err()
	}
	var out Contact
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		keepRow, err := getRow(ctx, q, keepID)
		if err != nil {
			return err
		}
		if int(keepRow.Version) != version {
			return apperr.StaleVersion("contact").WithDetail("currentVersion", keepRow.Version)
		}
		dupRow, err := q.GetContact(ctx, dupID)
		if database.IsNoRows(err) {
			var errs apperr.FieldErrors
			errs.Add("duplicateId", "no such contact")
			return errs.Err()
		}
		if err != nil {
			return err
		}
		if dupRow.CompanyID != keepRow.CompanyID {
			// Their jobs are at their own company, and it may be someone who changed jobs.
			var errs apperr.FieldErrors
			errs.Add("duplicateId", "works at another company: if it is the same person, change their company first")
			return errs.Err()
		}
		for _, m := range s.movers {
			if err := m.MoveContact(ctx, tx, dupID, keepID); err != nil {
				return err
			}
		}
		move := store.MoveEmploymentChecksParams{FromID: dupID, ToID: keepID}
		if err := q.MoveEmploymentChecks(ctx, move); err != nil {
			return err
		}
		if err := q.MoveLinkedinChecks(ctx, store.MoveLinkedinChecksParams(move)); err != nil {
			return err
		}
		if err := q.MoveMailboxChecks(ctx, store.MoveMailboxChecksParams(move)); err != nil {
			return err
		}
		keep, dup := fromRow(keepRow), fromRow(dupRow)
		after, filled := FillFrom(keep.Details, dup.Details)
		check := keep.EmailCheck
		if keep.Details.Email == nil && dup.Details.Email != nil {
			check = dup.EmailCheck // the email comes with its result
		}
		// The duplicate goes first: its email may be moving to the contact kept.
		if _, err := q.DeleteMergedContact(ctx, dupID); database.IsForeignKeyViolation(err) {
			var errs apperr.FieldErrors
			errs.Add("duplicateId", "has jobs or emails you cannot see: ask someone who sees them all to merge")
			return errs.Err()
		} else if err != nil {
			return err
		}
		params := store.UpdateContactParams{
			ID: keepID, ExpectedVersion: keepRow.Version,
			CompanyID: after.CompanyID, FirstName: after.FirstName, LastName: after.LastName, Title: after.Title,
			Email: after.Email, EmailCheckReasons: []string{}, Phone: after.Phone, Mobile: after.Mobile,
			LinkedinUrl: after.LinkedinURL, Status: after.Status, Role: after.Role, Notes: after.Notes,
		}
		if after.Email != nil && check != nil {
			params.EmailCheckStatus, params.EmailCheckReasons, params.EmailCheckedAt = &check.Status, check.Reasons, &check.CheckedAt
		}
		row, err := q.UpdateContact(ctx, params)
		if err != nil {
			return err
		}
		for _, e := range []audit.Entry{
			{Action: "contact.merged", EntityType: "contact", EntityID: keepID,
				Changes: map[string]any{"duplicateId": dupID, "duplicateName": strings.TrimSpace(dup.Details.FirstName + " " + dup.Details.LastName), "filled": filled}},
			{Action: "contact.merged_into", EntityType: "contact", EntityID: dupID, Changes: map[string]any{"keptId": keepID}},
		} {
			if err := audit.Record(ctx, tx, e); err != nil {
				return err
			}
		}
		out, err = s.withCompanyName(ctx, tx, fromRow(row))
		return err
	})
	return out, err
}
