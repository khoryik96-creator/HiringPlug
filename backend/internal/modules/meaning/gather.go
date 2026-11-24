package meaning

// Who a search looks at closely. When there are few enough to compare one
// by one (a small company, a user who may see only some of it, a folder or
// filters: at most ExactMax), everyone is compared exactly and none is
// missed. Otherwise the database's nearest-neighbour indexes gather the
// nearest by their titles, CV passages and profiles, and those whose CVs have
// the search's own words. An index search skips the rows the user may not
// see (another company's, out of their scope, in the Bin, already on the
// job) after finding them, so it looks wide, and with pgvector 0.8 or later
// it goes on until it has found enough.

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning/internal/store"
)

const (
	searchWidth = "1000"  // neighbours each index search keeps in view (pgvector's most)
	scanTuples  = "40000" // with iterative scans: rows an index search may go through
	nearTitles  = 200     // candidates gathered by their titles (each way)
	nearCVs     = 300     // ... by their CV passages
	nearWhole   = 150     // ... by their profiles
	titleRows   = 600     // rows looked at for them: one candidate may hold several near titles
	passageRows = 1000    // ... and several near passages
)

// gather returns who to look at: everyone when few enough, else the nearest
// by each index and those whose CVs have the words.
func (s *Service) gather(ctx context.Context, tx pgx.Tx, query store.QueryByHashRow, words []CVHit, ask Ask, sweep store.MeaningSweep) ([]string, error) {
	everyone, err := s.everyone(ctx, tx, ask, sweep)
	if err != nil {
		return nil, err
	}
	if everyone != nil {
		return withWords(leaveOut(everyone, ask.Exclude), words), nil
	}
	q := store.New(tx)
	if err := s.searchWidely(ctx, tx, q); err != nil {
		return nil, err
	}
	exclude := ask.Exclude
	if exclude == nil {
		exclude = []string{}
	}
	var ids []string
	seen := map[string]bool{}
	add := func(found []string, err error) error {
		for _, id := range found {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		return err
	}
	if query.HasRole {
		if err := add(q.NearByField(ctx, store.NearByFieldParams{QueryID: query.ID, Exclude: exclude, Rows: titleRows, N: nearTitles})); err != nil {
			return nil, err
		}
		if err := add(q.NearByWork(ctx, store.NearByWorkParams{QueryID: query.ID, Exclude: exclude, Rows: titleRows, N: nearTitles})); err != nil {
			return nil, err
		}
	}
	if err := add(q.NearByPassage(ctx, store.NearByPassageParams{QueryID: query.ID, Exclude: exclude, Rows: passageRows, N: nearCVs})); err != nil {
		return nil, err
	}
	profiles, err := q.NearByProfile(ctx, store.NearByProfileParams{QueryID: query.ID, Exclude: exclude, N: nearWhole})
	if err := add(profiles, err); err != nil {
		return nil, err
	}
	// Fewer than asked for, in a company too large to compare one by one:
	// most of it is out of the user's sight (their scope, another company on
	// the server). Those they may see, when few enough, are compared exactly.
	if len(profiles) < nearWhole && ask.Within == nil {
		visible, err := s.deps.Candidates.Visible(ctx, tx, ExactMax+1)
		if err != nil {
			return nil, err
		}
		if len(visible) <= ExactMax {
			return withWords(leaveOut(visible, ask.Exclude), words), nil
		}
	}
	if ask.Within != nil {
		ids = onlyIDs(ids, ask.Within)
	}
	return withWords(ids, words), nil
}

// everyone returns the candidates to compare one by one when there are few
// enough (nil: too many, use the indexes): the caller's own list, or
// everyone the user may see in a company of at most ExactMax.
func (s *Service) everyone(ctx context.Context, tx pgx.Tx, ask Ask, sweep store.MeaningSweep) ([]string, error) {
	if ask.Within != nil {
		if len(ask.Within) <= ExactMax {
			return ask.Within, nil
		}
		return nil, nil
	}
	if sweep.TotalCount == nil || int(*sweep.TotalCount) > ExactMax {
		return nil, nil
	}
	visible, err := s.deps.Candidates.Visible(ctx, tx, ExactMax+1)
	if err != nil || len(visible) > ExactMax {
		return nil, err
	}
	return visible, nil
}

// searchWidely widens the index searches; with pgvector 0.8 or later they
// also go on past the rows they skip until they have found enough.
func (s *Service) searchWidely(ctx context.Context, tx pgx.Tx, q *store.Queries) error {
	if err := q.SearchWidely(ctx, searchWidth); err != nil {
		return err
	}
	iterative, err := s.iterativeScans(ctx, tx)
	if err != nil || !iterative {
		return err
	}
	return q.SearchIteratively(ctx, scanTuples)
}

// iterativeScans says whether the database's pgvector has iterative index
// scans (0.8 or later); asked once.
func (s *Service) iterativeScans(ctx context.Context, tx pgx.Tx) (bool, error) {
	switch s.iterative.Load() {
	case 1:
		return true, nil
	case 2:
		return false, nil
	}
	var version string
	if err := tx.QueryRow(ctx, "SELECT coalesce((SELECT extversion FROM pg_extension WHERE extname = 'vector'), '')").Scan(&version); err != nil {
		return false, err
	}
	ok := versionAtLeast(version, 0, 8)
	if ok {
		s.iterative.Store(1)
	} else {
		s.iterative.Store(2)
	}
	return ok, nil
}

// versionAtLeast reads "0.8.1" (or "0.10") as at least major.minor.
func versionAtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return ma > major || (ma == major && mi >= minor)
}

// compare gathers, for each candidate, every comparison the ranking needs.
// Only readings made by the search's model are compared (vectors of another
// model mean nothing next to its own); those of older reading rules are,
// marked as such, until everyone is read again.
func (s *Service) compare(ctx context.Context, q *store.Queries, query store.QueryByHashRow, model string, ids []string, words []CVHit) (map[string]Evidence, error) {
	out := make(map[string]Evidence, len(ids))
	profiles, err := q.ProfileScores(ctx, store.ProfileScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return nil, err
	}
	read := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if Space(p.Model) != Space(model) {
			continue
		}
		e := out[p.MCandidateID]
		e.Profile, e.Read, e.SameRules = float64(p.Sim), true, p.Model == model
		out[p.MCandidateID] = e
		read = append(read, p.MCandidateID)
	}
	if len(read) > 0 {
		if err := s.compareRead(ctx, q, query, read, out); err != nil {
			return nil, err
		}
	}
	best := 0.0
	for _, h := range words {
		best = max(best, h.Rank)
	}
	for _, h := range words {
		e := out[h.CandidateID]
		if best > 0 {
			e.Words = h.Rank / best
		}
		e.WordsFound = WordsIn(h.Snippet)
		out[h.CandidateID] = e
	}
	return out, nil
}

// compareRead adds the titles, CV passages and a job's needs of those read.
func (s *Service) compareRead(ctx context.Context, q *store.Queries, query store.QueryByHashRow, ids []string, out map[string]Evidence) error {
	roles, err := q.RoleScores(ctx, store.RoleScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return err
	}
	for _, r := range roles {
		e := out[r.RCandidateID]
		e.Roles = append(e.Roles, RoleSim{
			Title: r.Title, Family: r.Family, Level: intOf(r.Level), EndYear: intOf(r.EndYear),
			// Read before titles said whether they were current: no end year was their job now.
			Now:   (r.Current != nil && *r.Current) || (r.Current == nil && r.EndYear == nil),
			Field: float64(r.FieldSim), Work: float64(r.WorkSim),
		})
		out[r.RCandidateID] = e
	}
	passages, err := q.PassageScores(ctx, store.PassageScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return err
	}
	for _, p := range passages {
		e := out[p.XCandidateID]
		e.PassageWhole, e.PassageContext, e.Passage = float64(p.WholeSim), float64(p.ContextSim), p.Passage
		out[p.XCandidateID] = e
	}
	if query.Needs == 0 {
		return nil
	}
	needs, err := q.NeedScores(ctx, store.NeedScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return err
	}
	for _, n := range needs {
		e := out[n.PCandidateID]
		e.Needs = append(e.Needs, float64(n.Sim))
		out[n.PCandidateID] = e
	}
	return nil
}

// leaveOut returns ids without those left out.
func leaveOut(ids, out []string) []string {
	if len(out) == 0 {
		return ids
	}
	drop := make(map[string]bool, len(out))
	for _, id := range out {
		drop[id] = true
	}
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if !drop[id] {
			kept = append(kept, id)
		}
	}
	return kept
}

// onlyIDs keeps the ids that are among allowed.
func onlyIDs(ids, allowed []string) []string {
	in := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		in[id] = true
	}
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if in[id] {
			kept = append(kept, id)
		}
	}
	return kept
}

// onlyWithin keeps the CV hits of candidates among allowed.
func onlyWithin(hits []CVHit, allowed []string) []CVHit {
	in := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		in[id] = true
	}
	var kept []CVHit
	for _, h := range hits {
		if in[h.CandidateID] {
			kept = append(kept, h)
		}
	}
	return kept
}

// withWords adds those whose CVs have the words (not already there).
func withWords(ids []string, words []CVHit) []string {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	for _, h := range words {
		if !seen[h.CandidateID] {
			seen[h.CandidateID] = true
			ids = append(ids, h.CandidateID)
		}
	}
	return ids
}
