package meaning

// Ranking candidates for a search or a job: the words are read once by the
// AI worker (a moment; the caller asks again while they are being read),
// then the database's nearest-neighbour indexes gather who to look at (by
// their titles, CV passages and profiles, and by the search's own words in
// their CVs), each of those is compared exactly, and domain.go ranks them.
// Everything runs as the user: the database leaves out the candidates they
// may not see.

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

const (
	rereadAfter = 2 * time.Minute // a search still being read after this is queued again (its job died)
	searchWidth = "400"           // neighbours each index search looks at
	nearTitles  = 200
	nearCVs     = 300
	nearWhole   = 150
	byWords     = 100
	passageMax  = 300 // characters of a passage shown
)

// Ranked is a candidate found by meaning, with why.
type Ranked struct {
	CandidateID string
	Score       float64
	Why         api.MeaningWhy
}

// Result is a ranking: how the words were read, the candidates found (best
// first), and those whose reading it compared and judged not to match, the
// words they share counted (a job's keyword matches it turned down).
type Result struct {
	Outcome    api.MeaningOutcome
	Found      []Ranked
	NotMatched []string
}

// Ask is a search or a job to rank candidates for.
type Ask struct {
	Title, Body string   // a job: its title and description; a search: its words (Body)
	Exclude     []string // candidates left out (those already on the job)
	Words       []CVHit  // a job's distinctive words found in CVs (the caller's keyword search)
	Limit       int
}

// Rank finds the candidates for a search or a job, best first, in the
// caller's transaction. While the words are being read the outcome says
// reading and nothing is ranked: ask again in a moment.
func (m *Module) Rank(ctx context.Context, tx pgx.Tx, p authz.Principal, ask Ask) (Result, error) {
	return m.service.rank(ctx, tx, p, ask)
}

func (s *Service) rank(ctx context.Context, tx pgx.Tx, p authz.Principal, ask Ask) (Result, error) {
	res := Result{Outcome: api.MeaningOutcome{Status: api.MeaningOutcomeStatusReading, Understood: nullable.NewNullNullable[api.MeaningUnderstood]()}}
	sweep, err := s.ensureRunning(ctx, tx, p.TenantID)
	if err != nil {
		return res, err
	}
	if err := s.progress(ctx, tx, &res.Outcome, sweep.Model); err != nil {
		return res, err
	}
	if sweep.UnavailableUntil != nil && sweep.UnavailableUntil.After(s.now()) {
		res.Outcome.Status = api.MeaningOutcomeStatusUnavailable
		return res, nil
	}
	model := ""
	if sweep.Model != nil {
		model = *sweep.Model
	}
	query, ready, err := s.readQuery(ctx, tx, p.TenantID, model, ask)
	if err != nil || !ready {
		return res, err
	}
	res.Outcome.Status = api.MeaningOutcomeStatusReady
	var understood api.MeaningUnderstood
	if len(query.Understood) > 0 && json.Unmarshal(query.Understood, &understood) == nil && string(query.Understood) != "null" {
		res.Outcome.Understood = nullable.NewNullableWithValue(understood)
	}
	res.Found, res.NotMatched, err = s.rankFor(ctx, tx, p, query, understood, ask)
	return res, err
}

// progress says how many candidates are read, of how many.
func (s *Service) progress(ctx context.Context, tx pgx.Tx, out *api.MeaningOutcome, model *string) error {
	total, err := s.deps.Candidates.Count(ctx, tx)
	if err != nil {
		return err
	}
	read := int64(0)
	if model != nil {
		if read, err = store.New(tx).CountRead(ctx, *model); err != nil {
			return err
		}
	}
	r := int(read)
	out.Read, out.Total = &r, &total
	return nil
}

// readQuery returns the search as read by the model that reads the
// company's candidates, or queues it to be read (ready false).
func (s *Service) readQuery(ctx context.Context, tx pgx.Tx, tenantID, model string, ask Ask) (store.QueryByHashRow, bool, error) {
	q := store.New(tx)
	hash := QueryHash(model, ask.Title, ask.Body)
	row, err := q.QueryByHash(ctx, hash)
	if database.IsNoRows(err) {
		var title *string
		if t := strings.TrimSpace(ask.Title); t != "" {
			t = clipText(t, 300)
			title = &t
		}
		ins, err := q.InsertQuery(ctx, store.InsertQueryParams{TextHash: hash, Title: title, Body: clipText(strings.TrimSpace(ask.Body), 20000)})
		if err != nil {
			return row, false, err
		}
		if ins.Status == "ready" { // read meanwhile by another request
			row, err = q.QueryByHash(ctx, hash)
			return row, err == nil, err
		}
		return row, false, s.queueQuery(ctx, tx, tenantID, ins.ID, title, ask.Body)
	}
	if err != nil {
		return row, false, err
	}
	if row.Status == "ready" {
		return row, true, q.TouchQuery(ctx, row.ID)
	}
	if row.CreatedAt.Before(s.now().Add(-rereadAfter)) { // its job died, or it failed: once more
		if err := q.RereadQuery(ctx, row.ID); err != nil {
			return row, false, err
		}
		text, err := q.QueryText(ctx, row.ID)
		if err != nil {
			return row, false, err
		}
		return row, false, s.queueQuery(ctx, tx, tenantID, row.ID, text.Title, text.Body)
	}
	return row, false, nil
}

func (s *Service) queueQuery(ctx context.Context, tx pgx.Tx, tenantID, id string, title *string, body string) error {
	_, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindMeaningReadQuery,
		api.MeaningReadQueryJob{QueryId: id, Title: title, Body: clipText(strings.TrimSpace(body), 20000)})
	return err
}

// rankFor gathers who to look at, compares each exactly and ranks them; it
// also says who it read and judged not to match.
func (s *Service) rankFor(ctx context.Context, tx pgx.Tx, p authz.Principal, query store.QueryByHashRow, understood api.MeaningUnderstood, ask Ask) ([]Ranked, []string, error) {
	q := store.New(tx)
	if err := q.SearchWidely(ctx, searchWidth); err != nil {
		return nil, nil, err
	}
	words := ask.Words
	if ask.Title == "" { // a typed search: its own words in CVs
		terms := []Term{}
		for _, w := range SearchWords(ask.Body) {
			terms = append(terms, Term{Text: w, Weight: 1})
		}
		if len(terms) > 0 {
			var err error
			if words, err = s.deps.CVs.SearchCVs(ctx, tx, terms, ask.Exclude, byWords); err != nil {
				return nil, nil, err
			}
		}
	}
	ids, err := s.gather(ctx, q, query, words, ask.Exclude)
	if err != nil || len(ids) == 0 {
		return nil, nil, err
	}
	evidence, err := s.compare(ctx, q, query, ids, words)
	if err != nil {
		return nil, nil, err
	}
	meta := Query{HasRole: query.HasRole, HasContext: query.HasContext, Describes: query.Describes, Exact: query.Exact, Needs: int(query.Needs)}
	if understood.Family != nil {
		meta.Family = *understood.Family
	}
	if understood.Near != nil {
		meta.Near = *understood.Near
	}
	if query.Level != nil {
		l := int(*query.Level)
		meta.Level = &l
	}
	year := s.now().Year()
	var out []Ranked
	var notMatched []string
	for _, id := range ids {
		e := evidence[id]
		scored := Score(meta, e, year)
		if scored.Score < Shown {
			if e.Read { // not someone it has yet to read
				notMatched = append(notMatched, id)
			}
			continue
		}
		out = append(out, Ranked{CandidateID: id, Score: scored.Score, Why: why(scored, e, p.Can(api.PermissionDocumentsRead))})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if ask.Limit > 0 && len(out) > ask.Limit {
		out = out[:ask.Limit]
	}
	return out, notMatched, nil
}

// gather returns who to look at: the nearest by each index, and those whose CVs have the words.
func (s *Service) gather(ctx context.Context, q *store.Queries, query store.QueryByHashRow, words []CVHit, exclude []string) ([]string, error) {
	seen := map[string]bool{}
	for _, id := range exclude {
		seen[id] = true
	}
	var ids []string
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
		if err := add(q.NearByField(ctx, store.NearByFieldParams{QueryID: query.ID, N: nearTitles})); err != nil {
			return nil, err
		}
		if err := add(q.NearByWork(ctx, store.NearByWorkParams{QueryID: query.ID, N: nearTitles})); err != nil {
			return nil, err
		}
	}
	if err := add(q.NearByPassage(ctx, store.NearByPassageParams{QueryID: query.ID, N: nearCVs})); err != nil {
		return nil, err
	}
	if err := add(q.NearByProfile(ctx, store.NearByProfileParams{QueryID: query.ID, N: nearWhole})); err != nil {
		return nil, err
	}
	for _, h := range words {
		if err := add([]string{h.CandidateID}, nil); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// compare gathers, for each candidate, every comparison the ranking needs.
func (s *Service) compare(ctx context.Context, q *store.Queries, query store.QueryByHashRow, ids []string, words []CVHit) (map[string]Evidence, error) {
	out := make(map[string]Evidence, len(ids))
	roles, err := q.RoleScores(ctx, store.RoleScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		e := out[r.RCandidateID]
		e.Roles = append(e.Roles, RoleSim{
			Title: r.Title, Family: r.Family, Level: intOf(r.Level), EndYear: intOf(r.EndYear),
			Field: float64(r.FieldSim), Work: float64(r.WorkSim),
		})
		out[r.RCandidateID] = e
	}
	passages, err := q.PassageScores(ctx, store.PassageScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return nil, err
	}
	for _, p := range passages {
		e := out[p.XCandidateID]
		e.PassageWhole, e.PassageContext, e.Passage = float64(p.WholeSim), float64(p.ContextSim), p.Passage
		out[p.XCandidateID] = e
	}
	profiles, err := q.ProfileScores(ctx, store.ProfileScoresParams{QueryID: query.ID, CandidateIds: ids})
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		e := out[p.MCandidateID]
		e.Profile, e.Read = float64(p.Sim), true
		out[p.MCandidateID] = e
	}
	if query.Needs > 0 {
		needs, err := q.NeedScores(ctx, store.NeedScoresParams{QueryID: query.ID, CandidateIds: ids})
		if err != nil {
			return nil, err
		}
		for _, n := range needs {
			e := out[n.PCandidateID]
			e.Needs = append(e.Needs, float64(n.Sim))
			out[n.PCandidateID] = e
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

// why says, in words, what found a candidate; their CV only to those who may read CVs.
func why(scored Scored, e Evidence, mayReadCVs bool) api.MeaningWhy {
	w := api.MeaningWhy{Score: float32(min(1, scored.Score)), Strength: api.MeaningWhyStrength(Strength(scored.Score)), Words: []string{}}
	if r := scored.Role; r != nil {
		title, when := r.Title, When(r.EndYear)
		w.Title, w.TitleWhen, w.Family = &title, &when, r.Family
		if r.Level != nil {
			if name, ok := LevelNames[*r.Level]; ok {
				w.Level = &name
			}
		}
	}
	if mayReadCVs {
		if scored.ShowPassage && e.Passage != "" {
			passage := clipText(e.Passage, passageMax)
			w.Passage = &passage
		}
		if e.WordsFound != nil {
			w.Words = e.WordsFound
		}
	}
	return w
}

func intOf(n *int16) *int {
	if n == nil {
		return nil
	}
	v := int(*n)
	return &v
}
