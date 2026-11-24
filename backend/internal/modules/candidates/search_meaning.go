package candidates

// Search by meaning in the advanced search: the meaning module ranks the
// candidates (wired by internal/app), and the search keeps the best that
// pass its other filters, in that order, each with why. A folder or filters
// that leave few enough candidates are applied first: only those are ranked
// (compared one by one, none missed).

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

const (
	// MeaningPool is how many of the best matches by meaning the other filters choose from.
	MeaningPool = 300
	// MeaningShown is how many a search by meaning shows (and exports), best first.
	MeaningShown = 100
	// meaningWithin is how many candidates a folder or filters may leave for
	// only those to be ranked (the meaning module compares as many one by one).
	meaningWithin = 2000
)

// Meaning ranks candidates by meaning, best first: among within when it is
// not nil.
type Meaning interface {
	Search(ctx context.Context, tx pgx.Tx, p authz.Principal, text string, within []string, limit int) (api.MeaningOutcome, []MeaningHit, error)
}

// MeaningHit is a candidate found by meaning, with why.
type MeaningHit struct {
	CandidateID string
	Why         api.MeaningWhy
}

// SearchByMeaningWith gives search the meaning module (wired after both exist).
func (m *Module) SearchByMeaningWith(mn Meaning) { m.service.meaning = mn }

// byMeaning ranks the search's meaning words (among within when not nil):
// ids is nil when nothing is ranked yet (still being read, or no model).
func (s *Service) byMeaning(ctx context.Context, tx pgx.Tx, p authz.Principal, text string, within []string, page *SearchPage) ([]string, map[string]api.MeaningWhy, error) {
	if s.meaning == nil {
		outcome := api.MeaningOutcome{Status: api.MeaningOutcomeStatusUnavailable}
		page.Meaning = &outcome
		return nil, nil, nil
	}
	outcome, hits, err := s.meaning.Search(ctx, tx, p, text, within, MeaningPool)
	page.Meaning = &outcome
	if err != nil || outcome.Status != api.MeaningOutcomeStatusReady {
		return nil, nil, err
	}
	ids := make([]string, len(hits))
	why := make(map[string]api.MeaningWhy, len(hits))
	for i, h := range hits {
		ids[i], why[h.CandidateID] = h.CandidateID, h.Why
	}
	return ids, why, nil
}

// narrows says whether a search has anything besides its meaning words that
// leaves candidates out (a folder, the owner, a filter).
func (f SearchFilter) narrows() bool {
	lists := [][]string{f.Availability, f.WorkKinds, f.Nationality, f.Residence, f.Sources, f.Industries, f.Skills, f.Tags, f.Specializations, f.Certifications}
	for _, l := range lists {
		if len(l) > 0 {
			return true
		}
	}
	set := func(v *string) bool { return v != nil && *v != "" }
	return set(f.Query) || set(f.Boolean) || set(f.Location) || set(f.SalaryMax) || set(f.Owner) || f.Folder != nil ||
		f.NoticeMaxDays != nil || f.UpdatedWithinDays != nil
}

// meaningWithin returns the candidates a search's folder and filters leave,
// for only those to be ranked by meaning; nil when nothing narrows it or more
// than meaningWithin are left (then the best matches of everyone are filtered).
func meaningWithinOf(ctx context.Context, q *store.Queries, f SearchFilter, in store.SearchCandidatesParams) ([]string, error) {
	if !f.narrows() {
		return nil, nil
	}
	in.Cursor, in.RowLimit = nil, meaningWithin+1
	rows, err := q.SearchCandidates(ctx, in)
	if err != nil || len(rows) > meaningWithin {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}

// keepOrder keeps the ids found by both, in the first one's order (a
// folder, when one is chosen, as well as a search by meaning).
func keepOrder(ranked, allowed []string) []string {
	if allowed == nil {
		return ranked
	}
	in := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		in[id] = true
	}
	out := []string{}
	for _, id := range ranked {
		if in[id] {
			out = append(out, id)
		}
	}
	return out
}

// inOrder puts rows in the order of ids.
func inOrder[T any](rows []T, ids []string, id func(T) string) []T {
	at := make(map[string]int, len(ids))
	for i, x := range ids {
		at[x] = i
	}
	slices.SortStableFunc(rows, func(a, b T) int { return at[id(a)] - at[id(b)] })
	return rows
}

// meaningReady refuses to go on with a search by meaning not ranked yet (an
// export of it would hold no one).
func meaningReady(outcome *api.MeaningOutcome) error {
	switch {
	case outcome == nil || outcome.Status == api.MeaningOutcomeStatusReady:
		return nil
	case outcome.Status == api.MeaningOutcomeStatusUnavailable:
		return apperr.New(api.ErrorCodeMEANINGUNAVAILABLE, "Search by meaning is not set up on this server.")
	}
	return apperr.New(api.ErrorCodeMEANINGREADING, "The search is still being read. Export again in a moment.")
}
