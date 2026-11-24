package meaning

// Reading each company's candidates in the background: the meaning.sweep job
// walks the candidates in id order, a batch at a time, and queues those
// whose titles, work or newest CV changed since they were read (their
// fingerprint), a few per AI job; then it starts again from the beginning
// after a rest, so nothing is missed. Each pass first counts the company's
// candidates and those read (what searches show as progress) and ends by
// forgetting searches not used for 90 days. A new CV, or one moved to or
// from the Bin, is read at once (cvChanged), not at the next pass. The job
// keeps itself going; a stalled one is restarted by the next search, status
// check or CV read (ensureRunning).

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oapi-codegen/nullable"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/meaning/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/redact"
)

const (
	sweepBatch  = 200              // candidates checked per run
	readBatch   = 5                // candidates per read job (well under a second's reading)
	maxPending  = 100              // reads queued and not back yet: wait for the worker to catch up
	sweepSoon   = 3 * time.Second  // the next run, while a pass goes on
	sweepWait   = 30 * time.Second // ... while the worker catches up
	sweepRest   = 10 * time.Minute // a pass done: start again after this
	recount     = 5 * time.Minute  // candidates and those read are counted again this often during a pass
	stalledFor  = time.Hour        // a run this late is taken as dead, and restarted
	unavailable = time.Hour        // the worker said it has no model: ask again then
	maxRoles    = 12               // the newest titles read (older ones say little)
	maxCVChars  = 30000
	maxTitle    = 200
	maxWork     = 4000
)

// Service holds the module's use cases.
type Service struct {
	pool      *pgxpool.Pool
	deps      Deps
	now       func() time.Time
	iterative atomic.Int32 // pgvector has iterative index scans: 0 not asked yet, 1 yes, 2 no
}

// sweep handles meaning.sweep: one batch of the company's read-through.
func (s *Service) sweep(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.MeaningSweepJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.GetSweep(ctx)
	if database.IsNoRows(err) || (err == nil && int(row.Chain) != payload.Chain) {
		return nil // an older run: the newer one goes on
	}
	if err != nil {
		return err
	}
	now := s.now()
	if row.UnavailableUntil != nil && row.UnavailableUntil.After(now) {
		return s.next(ctx, tx, job.TenantID, row.Chain, row.CursorID, false, *row.UnavailableUntil)
	}
	if err := q.ForgetStalePending(ctx); err != nil {
		return err
	}
	if n, err := q.CountPending(ctx); err != nil || n >= maxPending {
		if err != nil {
			return err
		}
		return s.next(ctx, tx, job.TenantID, row.Chain, row.CursorID, false, now.Add(sweepWait))
	}
	// A pass begins (or a few minutes went by): how far reading has got.
	if row.CursorID == "" || row.CountedAt == nil || row.CountedAt.Before(now.Add(-recount)) {
		if err := s.count(ctx, tx, row.Model); err != nil {
			return err
		}
	}
	sources, err := s.deps.Candidates.SourcesAfter(ctx, tx, row.CursorID, sweepBatch)
	if err != nil {
		return err
	}
	if len(sources) == 0 { // a pass is done
		if err := q.ForgetOldQueries(ctx); err != nil {
			return err
		}
		return s.next(ctx, tx, job.TenantID, row.Chain, "", true, now.Add(sweepRest))
	}
	if _, err := s.queueStale(ctx, tx, job.TenantID, sources, row.Model); err != nil {
		return err
	}
	return s.next(ctx, tx, job.TenantID, row.Chain, sources[len(sources)-1].CandidateID, false, now.Add(sweepSoon))
}

// next records a run and queues the next one.
func (s *Service) next(ctx context.Context, tx pgx.Tx, tenantID string, chain int32, cursor string, finished bool, due time.Time) error {
	done := int32(0)
	if finished {
		done = 1
	}
	if err := store.New(tx).SweepRan(ctx, store.SweepRanParams{CursorID: cursor, Finished: done, DueAt: due, Chain: chain}); err != nil {
		return err
	}
	_, err := jobqueue.EnqueueAt(ctx, tx, tenantID, api.JobKindMeaningSweep, api.MeaningSweepJob{Chain: int(chain)}, due)
	return err
}

// count notes how many of the company's candidates there are and how many
// the model reads them with has read (a search shows it while reading goes on).
func (s *Service) count(ctx context.Context, tx pgx.Tx, model *string) error {
	total, err := s.deps.Candidates.Count(ctx, tx)
	if err != nil {
		return err
	}
	q := store.New(tx)
	read := int64(0)
	if model != nil {
		if read, err = q.CountRead(ctx, *model); err != nil {
			return err
		}
	}
	return q.SetSweepCounts(ctx, store.SetSweepCountsParams{ReadCount: int32(min(read, int64(total))), TotalCount: int32(total)}) //nolint:gosec // counts of candidates
}

// ensureRunning starts the company's read-through if it has none, or again
// if its run is long overdue (its job died), in the caller's transaction. A
// plain read otherwise: searches never wait on one another here.
func (s *Service) ensureRunning(ctx context.Context, tx pgx.Tx, tenantID string) (store.MeaningSweep, error) {
	q := store.New(tx)
	row, err := q.GetSweep(ctx)
	if database.IsNoRows(err) {
		if row, err = q.StartSweep(ctx); database.IsNoRows(err) {
			return q.GetSweep(ctx) // made meanwhile by another request
		}
	} else if err != nil || row.DueAt.After(s.now().Add(-stalledFor)) {
		return row, err
	} else {
		if row.Chain, err = q.RestartSweep(ctx, false); err != nil {
			return row, err
		}
		logx.From(ctx).Info("search by meaning: read-through restarted", "chain", row.Chain)
	}
	if err != nil {
		return row, err
	}
	_, err = jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindMeaningSweep, api.MeaningSweepJob{Chain: int(row.Chain)})
	return row, err
}

// cvRead queues the candidate whose CV was just read, at once.
func (s *Service) cvRead(ctx context.Context, tx pgx.Tx, tenantID, documentID string) error {
	candidateID, err := s.deps.CVs.CandidateOf(ctx, tx, documentID)
	if err != nil || candidateID == "" {
		return err
	}
	return s.cvChanged(ctx, tx, tenantID, candidateID, documentID, false)
}

// cvChanged queues, at once, a candidate whose CV was just read, or moved to
// or from the Bin (a CV in the Bin is no longer read: what was read of it
// goes now).
func (s *Service) cvChanged(ctx context.Context, tx pgx.Tx, tenantID, candidateID, documentID string, binned bool) error {
	if binned {
		if err := store.New(tx).DeleteDocumentPassages(ctx, documentID); err != nil {
			return err
		}
	}
	// In a savepoint: the CV's own change is kept even if queueing this fails.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	row, err := s.ensureRunning(ctx, sp, tenantID)
	if err == nil && (row.UnavailableUntil == nil || !row.UnavailableUntil.After(s.now())) { // no model: nothing to queue
		var sources []Source
		if sources, err = s.deps.Candidates.Sources(ctx, sp, []string{candidateID}); err == nil {
			_, err = s.queueStale(ctx, sp, tenantID, sources, row.Model)
		}
	}
	if err != nil {
		logx.From(ctx).Warn("search by meaning: a candidate's changed CV not queued to be read", "candidate_id", candidateID, "error", err)
		return sp.Rollback(ctx)
	}
	return sp.Commit(ctx)
}

// queueStale queues the candidates whose fingerprint (or the model that
// read them) changed, and not already queued; it returns how many. Only
// their CVs' texts are loaded: those up to date are told by their CV's key.
func (s *Service) queueStale(ctx context.Context, tx pgx.Tx, tenantID string, sources []Source, model *string) (int, error) {
	sources, err := s.withCVKeys(ctx, tx, sources)
	if err != nil {
		return 0, err
	}
	ids := make([]string, len(sources))
	for i, src := range sources {
		ids[i] = src.CandidateID
	}
	q := store.New(tx)
	read, err := q.ReadOf(ctx, ids)
	if err != nil {
		return 0, err
	}
	pending, err := q.PendingOf(ctx, ids)
	if err != nil {
		return 0, err
	}
	have := map[string]store.ReadOfRow{}
	for _, r := range read {
		have[r.CandidateID] = r
	}
	queued := map[string]string{}
	for _, p := range pending {
		queued[p.CandidateID] = p.Fingerprint
	}
	var staleSources []Source
	var withCV []string
	for _, src := range sources {
		fp := Fingerprint(src)
		r, ok := have[src.CandidateID]
		upToDate := ok && r.Fingerprint == fp && (model == nil || r.Model == *model)
		if upToDate || queued[src.CandidateID] == fp {
			continue
		}
		staleSources = append(staleSources, src)
		if src.CV != nil {
			withCV = append(withCV, src.CandidateID)
		}
	}
	texts := map[string]CV{}
	if len(withCV) > 0 {
		if texts, err = s.deps.CVs.NewestCVs(ctx, tx, withCV); err != nil {
			return 0, err
		}
	}
	stale := make([]api.MeaningCandidateSource, 0, len(staleSources))
	for _, src := range staleSources {
		var text *CV
		if cv, ok := texts[src.CandidateID]; ok && src.CV != nil && cv.DocumentID == src.CV.DocumentID {
			text = &cv
		}
		stale = append(stale, sourceForAI(src, Fingerprint(src), text))
	}
	for start := 0; start < len(stale); start += readBatch {
		batch := stale[start:min(start+readBatch, len(stale))]
		if _, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindMeaningReadCandidates, api.MeaningReadCandidatesJob{Candidates: batch}); err != nil {
			return 0, err
		}
		ids, fps := make([]string, len(batch)), make([]string, len(batch))
		for i, c := range batch {
			ids[i], fps[i] = c.CandidateId, c.Fingerprint
		}
		if err := q.MarkPending(ctx, store.MarkPendingParams{CandidateIds: ids, Fingerprints: fps}); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

// withCVKeys adds each candidate's newest CV key (which CV, which reading of it).
func (s *Service) withCVKeys(ctx context.Context, tx pgx.Tx, sources []Source) ([]Source, error) {
	ids := make([]string, len(sources))
	for i, src := range sources {
		ids[i] = src.CandidateID
	}
	keys, err := s.deps.CVs.NewestCVKeys(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Source, len(sources))
	for i, src := range sources {
		if k, ok := keys[src.CandidateID]; ok {
			src.CV = &k
		}
		out[i] = src
	}
	return out, nil
}

// sourceForAI is what the AI worker is sent of a candidate: their titles,
// newest first (their title now first when their work history does not
// start with it), what they did in each, and their newest CV without salary.
// A past job with no end date ended no later than the next one began.
func sourceForAI(src Source, fingerprint string, cv *CV) api.MeaningCandidateSource {
	out := api.MeaningCandidateSource{CandidateId: src.CandidateID, Fingerprint: fingerprint, Roles: []api.MeaningRoleSource{}}
	work := append([]Work(nil), src.Work...)
	sort.SliceStable(work, func(i, j int) bool { return work[i].Position < work[j].Position })
	seen := map[string]bool{}
	add := func(title, description string, end *int, current bool) {
		title = clipText(strings.TrimSpace(title), maxTitle)
		key := strings.ToLower(title)
		if title == "" || seen[key] || len(out.Roles) == maxRoles {
			return
		}
		seen[key] = true
		role := api.MeaningRoleSource{Ordinal: len(out.Roles), Title: title, Current: current}
		if end != nil {
			role.EndYear = nullable.NewNullableWithValue(*end)
		}
		if d := strings.TrimSpace(redact.WithoutSalary(description)); d != "" {
			d = clipText(d, maxWork)
			role.Description = &d
		}
		out.Roles = append(out.Roles, role)
	}
	if t := strings.TrimSpace(src.CurrentTitle); t != "" && (len(work) == 0 || !strings.EqualFold(t, strings.TrimSpace(work[0].Title))) {
		add(t, "", nil, true)
	}
	var newerStart *int // when the job after this one (newer) began
	for _, w := range work {
		end := yearOf(w.EndDate)
		if !w.Current && end == nil {
			end = newerStart // ended, when unknown: no later than the next job began
		}
		if !w.Current && end == nil {
			end = yearOf(w.StartDate) // ... or, failing that, counted from when it began
		}
		now := w.Current || (end == nil && w.Position == 0 && newerStart == nil)
		if now {
			end = nil
		}
		add(w.Title, w.Description, end, now)
		if start := yearOf(w.StartDate); start != nil {
			newerStart = start
		}
	}
	if cv != nil {
		out.Cv = &api.MeaningCVSource{DocumentId: cv.DocumentID, Text: clipText(redact.WithoutSalary(cv.Text), maxCVChars)}
	}
	return out
}

// yearOf reads "2019" or "2019-04"; nil when there is none.
func yearOf(date string) *int {
	if len(date) < 4 {
		return nil
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return nil
	}
	return &y
}

// clipText cuts s to at most n characters (never inside a character).
func clipText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
