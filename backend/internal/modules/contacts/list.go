package contacts

// The contact list, and the same list in the caller's transaction (exports).

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/boolsearch"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// List returns up to limit contacts matching f, newest first.
func (s *Service) List(ctx context.Context, p authz.Principal, f Filter, cursor *string, limit int) (Page, error) {
	var page Page
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		page, err = s.listTx(ctx, tx, p, f, cursor, limit)
		return err
	})
	return page, err
}

func (s *Service) listTx(ctx context.Context, tx pgx.Tx, p authz.Principal, f Filter, cursor *string, limit int) (Page, error) {
	var page Page
	tsquery, err := boolsearch.TSQueryOf("boolean", f.Boolean)
	if err != nil {
		return page, err
	}
	similar := f.Similar && f.Query != nil
	if similar {
		cursor = nil // ordered by likeness, not by id: one page
	}
	inIDs, err := s.folderItems.IDs(ctx, tx, p, f.Folder)
	if err != nil {
		return page, err
	}
	var untouchedSince *time.Time
	if f.Stale {
		since := StaleSince(time.Now())
		untouchedSince = &since
	}
	rows, err := store.New(tx).ListContacts(ctx, store.ListContactsParams{
		Cursor: cursor, CompanyID: f.CompanyID, Query: f.Query, Tsquery: tsquery, Status: f.Status, Role: f.Role, RoleNotSet: f.Role == nil && f.RoleNotSet, InIds: inIDs, UntouchedSince: untouchedSince,
		SimilarNames: similar, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
	})
	if err != nil {
		return page, err
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
		return page, err
	}
	page.Items = make([]Summary, len(rows))
	for i, r := range rows {
		page.Items[i] = Summary{
			ID: r.ID, CompanyID: r.CompanyID, CompanyName: names[r.CompanyID], FirstName: r.FirstName, LastName: r.LastName,
			Status: r.Status, Title: r.Title, Email: r.Email, EmailCheckStatus: r.EmailCheckStatus, MailboxStatus: r.MailboxStatus,
			Phone: r.Phone, Mobile: r.Mobile, Role: r.Role, UpdatedAt: r.UpdatedAt, TouchedAt: r.TouchedAt,
		}
		page.Items[i].MailboxCheckedAt = r.MailboxCheckedAt
		if r.LinkedinCheckedAt != nil {
			page.Items[i].Linkedin = &LinkedinEmployment{Current: r.LinkedinCurrent, Joined: r.LinkedinJoined, Left: r.LinkedinLeft,
				MatchedByName: r.LinkedinMatchedByName, CheckedAt: *r.LinkedinCheckedAt}
		}
	}
	return page, nil
}
