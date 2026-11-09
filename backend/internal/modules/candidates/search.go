package candidates

// Advanced search: filters, and a boolean search (SAP AND (BW OR BPC) NOT
// internship) of the candidates' CVs and profiles.

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/boolsearch"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// CVs is what search needs from the documents module: the candidates whose
// CVs match a boolean search (as a text search query), and where.
type CVs interface {
	CandidatesWithMatchingCV(ctx context.Context, tx pgx.Tx, ts string, limit int) ([]string, error)
	CVSnippets(ctx context.Context, tx pgx.Tx, ts string, candidateIDs []string) (map[string]string, error)
}

// SearchCVsWith gives search the documents module (wired after both exist).
func (m *Module) SearchCVsWith(c CVs) { m.service.cvs = c }

// MaxCVMatches is the most candidates a boolean search finds by their CVs.
const MaxCVMatches = 20000

// SearchFilter is an advanced search: every part given must hold.
type SearchFilter struct {
	Query, Boolean                                  *string
	Availability, WorkKinds, Nationality, Residence []string
	Sources, Industries, Skills, Tags               []string
	AllSkills                                       bool
	Location                                        *string
	NoticeMaxDays, UpdatedWithinDays                *int
	SalaryMax, SalaryCurrency                       *string // per month
	Owner                                           *string // a user id
}

// SearchHit is a candidate found, with where the boolean matched.
type SearchHit struct {
	Summary
	InCV, InProfile bool
	CVSnippet       *string // the words found between [[ and ]]
}

// SearchPage is a page of hits, and the boolean's words to highlight.
type SearchPage struct {
	Items      []SearchHit
	NextCursor *string
	Terms      []string
}

// Search runs an advanced search, newest first.
func (s *Service) Search(ctx context.Context, p authz.Principal, f SearchFilter, cursor *string, limit int) (SearchPage, error) {
	var page SearchPage
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		page, err = s.searchTx(ctx, tx, p, f, cursor, limit)
		return err
	})
	return page, err
}

// SearchIn runs an advanced search in the caller's transaction (the AI
// candidate search picks who to consider with it): the first limit found.
func (m *Module) SearchIn(ctx context.Context, tx pgx.Tx, p authz.Principal, f SearchFilter, limit int) (SearchPage, error) {
	return m.service.searchTx(ctx, tx, p, f, nil, limit)
}

func (s *Service) searchTx(ctx context.Context, tx pgx.Tx, p authz.Principal, f SearchFilter, cursor *string, limit int) (SearchPage, error) {
	var page SearchPage
	var ts *string
	excluded := ""
	if f.Boolean != nil {
		q, err := boolsearch.Parse(*f.Boolean)
		if err != nil {
			return page, apperr.Validation(apperr.FieldErrors{{Field: "boolean", Message: err.Error()}})
		}
		t := q.TSQuery()
		ts, excluded = &t, q.Excluded()
		for _, term := range q.Terms {
			page.Terms = append(page.Terms, term.Text())
		}
	}
	if f.SalaryMax != nil && !canSeeSalary(p) {
		return page, apperr.Validation(apperr.FieldErrors{{Field: "expectedSalaryMax", Message: "needs a role that may see salaries"}})
	}
	inCV := map[string]bool{}
	var cvIDs, excludedIDs []string
	if ts != nil && s.cvs != nil {
		var err error
		if cvIDs, err = s.cvs.CandidatesWithMatchingCV(ctx, tx, *ts, MaxCVMatches); err != nil {
			return page, err
		}
		if excluded != "" { // a word left out with NOT, in their CV, leaves them out
			if excludedIDs, err = s.cvs.CandidatesWithMatchingCV(ctx, tx, excluded, MaxCVMatches); err != nil {
				return page, err
			}
		}
		for _, id := range cvIDs {
			inCV[id] = true
		}
	}
	notice, updated := int32Ptr(f.NoticeMaxDays), int32Ptr(f.UpdatedWithinDays)
	rows, err := store.New(tx).SearchCandidates(ctx, store.SearchCandidatesParams{
		Ts: ts, Cursor: cursor, Query: f.Query, Phones: s.queryPhones(f.Query), CvIds: orEmpty(cvIDs), ExcludedIds: orEmpty(excludedIDs),
		Availability: orEmpty(f.Availability), WaKinds: orEmpty(f.WorkKinds), Nationality: orEmpty(f.Nationality),
		Residence: orEmpty(f.Residence), Sources: orEmpty(f.Sources), Industries: orEmpty(f.Industries),
		Skills: orEmpty(f.Skills), AllSkills: f.AllSkills, Tags: orEmpty(f.Tags), Location: f.Location,
		NoticeMaxDays: notice, SalaryMax: f.SalaryMax, SalaryCurrency: f.SalaryCurrency, Owner: f.Owner,
		UpdatedWithinDays: updated, RowLimit: int32(limit + 1), //nolint:gosec // limit is 1..100 per the contract
	})
	if err != nil {
		return page, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1].ID
		page.NextCursor = &last
	}
	summaries := make([]Summary, len(rows))
	var matched []string
	for i, r := range rows {
		summaries[i] = summaryOf(store.ListCandidatesRow{
			ID: r.ID, FirstName: r.FirstName, LastName: r.LastName, Email: r.Email, Phone: r.Phone,
			CurrentTitle: r.CurrentTitle, CurrentCompany: r.CurrentCompany, Location: r.Location,
			WorkAuthorization: r.WorkAuthorization, Tags: r.Tags, UpdatedAt: r.UpdatedAt, Availability: r.Availability,
			AvailabilitySince: r.AvailabilitySince, NoticePeriodValue: r.NoticePeriodValue, NoticePeriodUnit: r.NoticePeriodUnit,
			WorkAuthorizationBasis: r.WorkAuthorizationBasis, Nationality: r.Nationality, ResidenceCountry: r.ResidenceCountry,
			Source: r.Source, Industry: r.Industry,
		}, !canSeeSalary(p))
		if inCV[r.ID] {
			matched = append(matched, r.ID)
		}
	}
	if err := attachCompensation(ctx, tx, p, summaries); err != nil {
		return page, err
	}
	snippets := map[string]string{}
	if len(matched) > 0 {
		if snippets, err = s.cvs.CVSnippets(ctx, tx, *ts, matched); err != nil {
			return page, err
		}
	}
	page.Items = make([]SearchHit, len(rows))
	for i, r := range rows {
		page.Items[i] = SearchHit{Summary: summaries[i], InCV: inCV[r.ID], InProfile: r.ProfileHit}
		if sn, ok := snippets[r.ID]; ok && strings.TrimSpace(sn) != "" {
			page.Items[i].CVSnippet = &sn
		}
	}
	return page, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func int32Ptr(n *int) *int32 {
	if n == nil {
		return nil
	}
	v := int32(*n) //nolint:gosec // bounded by the contract
	return &v
}
