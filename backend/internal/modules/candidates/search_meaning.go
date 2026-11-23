package candidates

// Search by meaning in the advanced search: the meaning module ranks the
// candidates (wired by internal/app), and the search keeps the best that
// pass its other filters, in that order, each with why.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// MeaningPool is how many of the best matches by meaning the other filters choose from.
const MeaningPool = 300

// Meaning ranks candidates by meaning, best first.
type Meaning interface {
	Search(ctx context.Context, tx pgx.Tx, p authz.Principal, text string, limit int) (api.MeaningOutcome, []MeaningHit, error)
}

// MeaningHit is a candidate found by meaning, with why.
type MeaningHit struct {
	CandidateID string
	Why         api.MeaningWhy
}

// SearchByMeaningWith gives search the meaning module (wired after both exist).
func (m *Module) SearchByMeaningWith(mn Meaning) { m.service.meaning = mn }

// byMeaning ranks the search's meaning words: ids is nil when nothing is
// ranked yet (still being read, or no model).
func (s *Service) byMeaning(ctx context.Context, tx pgx.Tx, p authz.Principal, text string, page *SearchPage) ([]string, map[string]api.MeaningWhy, error) {
	if s.meaning == nil {
		outcome := api.MeaningOutcome{Status: api.MeaningOutcomeStatusUnavailable}
		page.Meaning = &outcome
		return nil, nil, nil
	}
	outcome, hits, err := s.meaning.Search(ctx, tx, p, text, MeaningPool)
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
