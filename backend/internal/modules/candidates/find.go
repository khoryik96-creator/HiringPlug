package candidates

// The home page search, by Kano's /find rules (see platform/findquery):
// several searches at once, each by name, email or whole phone number,
// optionally among those who work or worked at a company.

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/findquery"
)

// findShown is how many candidates one search shows, matches and names
// alike together; findPool is how many are looked at for that.
const (
	findShown = 10
	findPool  = 200
)

// Found is a candidate one search found.
type Found struct {
	ID, FullName                 string
	CurrentTitle, CurrentCompany *string
	Email, Phone                 *string
	Employer                     *Employer // with a company searched for
	Score                        float64   // for names alike
}

// Employer is where a candidate found works or worked.
type Employer struct {
	Company string
	Title   *string
	Current bool
}

// FindResult is one search's block.
type FindResult struct {
	findquery.Search
	Items, Similar []Found
	More           bool
	Note           string
}

// Find runs every search in q (at most findquery.MaxSearches; skipped
// says how many more there were).
func (s *Service) Find(ctx context.Context, p authz.Principal, q string) (results []FindResult, skipped int, err error) {
	texts, skipped := findquery.Split(q)
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		results = make([]FindResult, len(texts))
		for i, text := range texts {
			r, err := s.findOne(ctx, store.New(tx), findquery.Parse(text))
			if err != nil {
				return err
			}
			results[i] = r
		}
		return nil
	})
	return results, skipped, err
}

func (s *Service) findOne(ctx context.Context, q *store.Queries, search findquery.Search) (FindResult, error) {
	r := FindResult{Search: search}
	if search.Error != "" {
		return r, nil
	}
	params := store.FindCandidatesParams{Phones: []string{}, Probes: []string{}, CompanyWords: likePatterns(findquery.CompanyWords(search.Company)), RowLimit: findPool}
	switch search.Kind {
	case findquery.Phone:
		params.Phones = findquery.Numbers(search.Value, s.phoneRegion)
		if len(params.Phones) == 0 {
			return r, nil
		}
	case findquery.Email:
		params.Email = &search.Value
	case findquery.Name:
		params.Name = search.Value
	}
	items, err := s.findRows(ctx, q, params, search.Company)
	if err != nil {
		return r, err
	}
	switch search.Kind {
	case findquery.Phone:
		// The likeliest reading of the number first (the agency's country, then
		// the usual candidate countries).
		slices.SortStableFunc(items, func(a, b Found) int {
			return cmp.Compare(slices.Index(params.Phones, deref(a.Phone)), slices.Index(params.Phones, deref(b.Phone)))
		})
		r.Note = countryNote(search.Value, items)
	case findquery.Name:
		if len(items) < findShown {
			if r.Similar, err = s.similarNames(ctx, q, params, search, items); err != nil {
				return r, err
			}
		}
	}
	r.More = len(items) > findShown
	r.Items = items[:min(len(items), findShown)]
	return r, nil
}

// findRows are the candidates params finds, kept to those who work or
// worked at company (word for word) when there is one.
func (s *Service) findRows(ctx context.Context, q *store.Queries, params store.FindCandidatesParams, company string) ([]Found, error) {
	rows, err := q.FindCandidates(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]Found, len(rows))
	ids := make([]string, len(rows))
	for i, r := range rows {
		out[i] = Found{
			ID: r.ID, FullName: FullName(Profile{FirstName: r.FirstName, LastName: r.LastName}),
			CurrentTitle: r.CurrentTitle, CurrentCompany: r.CurrentCompany, Email: r.Email, Phone: r.Phone,
		}
		ids[i] = r.ID
	}
	if company == "" || len(out) == 0 {
		return out, nil
	}
	work, err := q.ListWorkCompanies(ctx, ids)
	if err != nil {
		return nil, err
	}
	byCandidate := map[string][]store.ListWorkCompaniesRow{}
	for _, w := range work {
		byCandidate[w.CandidateID] = append(byCandidate[w.CandidateID], w)
	}
	kept := out[:0]
	for _, f := range out {
		if e := employerOf(f, byCandidate[f.ID], company); e != nil {
			f.Employer = e
			kept = append(kept, f)
		}
	}
	return kept, nil
}

// employerOf is where f works or worked at company: their current company
// first, then their work history, newest first.
func employerOf(f Found, work []store.ListWorkCompaniesRow, company string) *Employer {
	if f.CurrentCompany != nil && findquery.WorksAt(*f.CurrentCompany, company) {
		return &Employer{Company: *f.CurrentCompany, Title: f.CurrentTitle, Current: true}
	}
	for _, w := range work {
		if findquery.WorksAt(w.Company, company) {
			return &Employer{Company: w.Company, Title: w.Title, Current: w.IsCurrent}
		}
	}
	return nil
}

// similarNames fills the rest of the block with names alike, best first.
func (s *Service) similarNames(ctx context.Context, q *store.Queries, params store.FindCandidatesParams, search findquery.Search, found []Found) ([]Found, error) {
	params.SimilarNames = true
	params.Probes = likePatterns(findquery.NameProbes(search.Value))
	pool, err := s.findRows(ctx, q, params, search.Company)
	if err != nil {
		return nil, err
	}
	var alike []Found
	for _, f := range pool {
		if slices.ContainsFunc(found, func(x Found) bool { return x.ID == f.ID }) {
			continue
		}
		if f.Score = findquery.NameScore(search.Value, f.FullName); f.Score >= findquery.MinNameScore {
			alike = append(alike, f)
		}
	}
	slices.SortStableFunc(alike, func(a, b Found) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), cmp.Compare(len(a.FullName), len(b.FullName)), strings.Compare(a.ID, b.ID))
	})
	return alike[:min(len(alike), findShown-len(found))], nil
}

// countryNote warns when a number typed without its country code found
// numbers of different countries.
func countryNote(typed string, items []Found) string {
	if findquery.HasCountryCode(typed) {
		return ""
	}
	codes := map[int32]bool{}
	for _, f := range items {
		codes[findquery.CountryCode(deref(f.Phone))] = true
	}
	if len(codes) < 2 {
		return ""
	}
	return "No country code was typed, and numbers in more than one country match: check the number on each profile, or type it with + and the country code."
}

// queryPhones are the whole numbers a list search can be, when it is a
// phone number (none otherwise).
func (s *Service) queryPhones(query *string) []string {
	if query != nil {
		if search := findquery.Parse(*query); search.Kind == findquery.Phone && search.Error == "" && search.Company == "" {
			if numbers := findquery.Numbers(search.Value, s.phoneRegion); numbers != nil {
				return numbers
			}
		}
	}
	return []string{}
}

// likePatterns are LIKE patterns finding each word anywhere.
func likePatterns(words []string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(w) + "%"
	}
	return out
}
