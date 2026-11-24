package meaning

// Ranking candidates for a search or a job: the words are read once by the
// AI worker (a moment; the caller asks again while they are being read),
// then who to look at is gathered (gather.go), each of those is compared
// exactly, and domain.go ranks them. Everything runs as the user: the
// database leaves out the candidates they may not see.

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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

const (
	rereadAfter = 2 * time.Minute // a search still being read after this is queued again (its job died)
	byWords     = 100             // candidates found by the search's own words in their CVs
	passageMax  = 300             // characters of a passage shown
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
	Within      []string // only these candidates (a folder or filters), when not nil
	Limit       int
}

// ExactMax is how many candidates a search compares one by one at most: a
// company (or what the user may see, or a folder or filters) of at most this
// many is compared exactly, none missed; a larger one through the indexes.
const ExactMax = 2000

// Rank finds the candidates for a search or a job, best first, in the
// caller's transaction. While the words are being read the outcome says
// reading and nothing is ranked: ask again in a moment.
func (m *Module) Rank(ctx context.Context, tx pgx.Tx, p authz.Principal, ask Ask) (Result, error) {
	return m.service.rank(ctx, tx, p, ask)
}

func (s *Service) rank(ctx context.Context, tx pgx.Tx, p authz.Principal, ask Ask) (Result, error) {
	res := Result{Outcome: api.MeaningOutcome{Status: api.MeaningOutcomeStatusReading, Understood: nullable.NewNullNullable[api.MeaningUnderstood]()}}
	ask.Title, ask.Body = withoutMoney(ask.Title), strings.TrimSpace(redact.WithoutSalary(ask.Body))
	if ask.Title == "" { // a typed search: any amount goes, salary word or not
		ask.Body = withoutMoney(ask.Body)
	}
	sweep, err := s.ensureRunning(ctx, tx, p.TenantID)
	if err != nil {
		return res, err
	}
	if err := s.progress(ctx, tx, &res.Outcome, sweep); err != nil {
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
	res.Found, res.NotMatched, err = s.rankFor(ctx, tx, p, query, understood, ask, sweep, model)
	return res, err
}

var placeholders = strings.NewReplacer("[amount removed]", " ", "[salary removed]", " ")

// withoutMoney takes any amount of money out of a short text someone typed.
func withoutMoney(text string) string {
	return strings.Join(strings.Fields(placeholders.Replace(redact.WithoutMoney(text))), " ")
}

// progress says how many of the company's candidates are read, of how many,
// as the read-through counted them (counting the company on every search
// would take seconds in a large one); before its first count, counted now.
func (s *Service) progress(ctx context.Context, tx pgx.Tx, out *api.MeaningOutcome, sweep store.MeaningSweep) error {
	if sweep.TotalCount != nil {
		total, read := int(*sweep.TotalCount), 0
		if sweep.ReadCount != nil {
			read = min(int(*sweep.ReadCount), total)
		}
		out.Read, out.Total = &read, &total
		return nil
	}
	total, err := s.deps.Candidates.Count(ctx, tx)
	if err != nil {
		return err
	}
	read := int64(0)
	if sweep.Model != nil {
		if read, err = store.New(tx).CountRead(ctx, *sweep.Model); err != nil {
			return err
		}
	}
	r := min(int(read), total)
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
		ins, err := q.InsertQuery(ctx, store.InsertQueryParams{TextHash: hash, Title: title, Body: clipText(ask.Body, 20000)})
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
// also says who it read and judged not to match: by a title they hold, read
// by the rules that read the search (someone with no title, or not yet read
// again by new rules, is not turned down).
func (s *Service) rankFor(ctx context.Context, tx pgx.Tx, p authz.Principal, query store.QueryByHashRow, understood api.MeaningUnderstood,
	ask Ask, sweep store.MeaningSweep, model string,
) ([]Ranked, []string, error) {
	words := ask.Words
	if ask.Title == "" { // a typed search: its own words in CVs
		if terms := SearchTerms(ask.Body); len(terms) > 0 {
			var err error
			if words, err = s.deps.CVs.SearchCVs(ctx, tx, terms, ask.Exclude, byWords); err != nil {
				return nil, nil, err
			}
		}
	}
	if ask.Within != nil { // the caller's folder or filters: only those
		words = onlyWithin(words, ask.Within)
	}
	ids, err := s.gather(ctx, tx, query, words, ask, sweep)
	if err != nil || len(ids) == 0 {
		return nil, nil, err
	}
	evidence, err := s.compare(ctx, store.New(tx), query, model, ids, words)
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
			if e.Read && e.SameRules && len(e.Roles) > 0 {
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

// why says, in words, what found a candidate; their CV only to those who may read CVs.
func why(scored Scored, e Evidence, mayReadCVs bool) api.MeaningWhy {
	w := api.MeaningWhy{Score: float32(min(1, scored.Score)), Strength: api.MeaningWhyStrength(Strength(scored.Score)), Words: []string{}}
	if r := scored.Role; r != nil {
		title, when := r.Title, When(*r)
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
