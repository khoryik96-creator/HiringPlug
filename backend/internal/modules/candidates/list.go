package candidates

// Listing candidates: a page of summaries, with their status, notice period
// and, for those who may see them, salaries.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Summary is a candidate as shown in lists.
type Summary struct {
	ID                      string
	FullName                string
	Email                   *string
	Phone                   *string
	CurrentTitle            *string
	CurrentCompany          *string
	Location                *string
	WorkAuthorizationStatus *string
	Tags                    []string
	UpdatedAt               time.Time
	Availability            *string
	AvailabilitySince       *time.Time
	NoticePeriod            *NoticePeriod
	Compensation            *Compensation // nil when none is recorded or the user may not see it
	SalaryRestricted        bool          // the user may not see salaries
}

// Page is one page of a list, newest first.
type Page struct {
	Items      []Summary
	NextCursor *string
}

// Filter narrows a list: text, tag and skill (each optional).
type Filter struct {
	Query, Tag, Skill *string
	Owner             *string // a user id
	// Similar: names like Query (typos), the closest first, one page only.
	Similar bool
}

// List returns up to limit candidates matching f, starting after cursor (the
// last ID of the previous page).
func (s *Service) List(ctx context.Context, p authz.Principal, f Filter, cursor *string, limit int) (Page, error) {
	var page Page
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		similar := f.Similar && f.Query != nil
		if similar {
			cursor = nil // ordered by likeness, not by id: one page
		}
		rows, err := store.New(tx).ListCandidates(ctx, store.ListCandidatesParams{
			Cursor: cursor, Query: f.Query, Tag: f.Tag, Skill: f.Skill, Owner: f.Owner, SimilarNames: similar, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
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
		page.Items = make([]Summary, len(rows))
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
			page.Items[i] = Summary{
				ID: r.ID, FullName: FullName(Profile{FirstName: r.FirstName, LastName: r.LastName}),
				Email: r.Email, Phone: r.Phone, CurrentTitle: r.CurrentTitle, CurrentCompany: r.CurrentCompany,
				Location: r.Location, WorkAuthorizationStatus: r.WorkAuthorization, Tags: r.Tags, UpdatedAt: r.UpdatedAt,
				Availability: r.Availability, AvailabilitySince: r.AvailabilitySince, SalaryRestricted: !canSeeSalary(p),
			}
			if r.NoticePeriodValue != nil && r.NoticePeriodUnit != nil {
				page.Items[i].NoticePeriod = &NoticePeriod{Value: int(*r.NoticePeriodValue), Unit: *r.NoticePeriodUnit}
			}
		}
		if !canSeeSalary(p) || len(ids) == 0 {
			return nil
		}
		// The database shows salaries only with candidates.compensation:read too.
		comps, err := store.New(tx).ListCompensations(ctx, ids)
		if err != nil {
			return err
		}
		byID := make(map[string]store.CandidateCompensation, len(comps))
		for _, c := range comps {
			byID[c.CandidateID] = c
		}
		for i := range page.Items {
			if row, ok := byID[page.Items[i].ID]; ok {
				c, err := fromCompensationRow(row)
				if err != nil {
					return err
				}
				page.Items[i].Compensation = &c
			}
		}
		return nil
	})
	return page, err
}
