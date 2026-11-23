package candidates

// What search by meaning reads of candidates (the meaning module, through
// internal/app): their title now and their work history.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
)

// MeaningWork is one job a candidate held.
type MeaningWork struct {
	Position                                   int
	Title, Company, StartDate, EndDate, Detail string
	Current                                    bool
}

// MeaningSource is a candidate's title now and their work, newest first.
type MeaningSource struct {
	ID, CurrentTitle string
	Work             []MeaningWork
}

// MeaningSourcesAfter returns the candidates after cursorID in id order (at
// most limit), in the caller's transaction.
func (m *Module) MeaningSourcesAfter(ctx context.Context, tx pgx.Tx, cursorID string, limit int) ([]MeaningSource, error) {
	q := store.New(tx)
	rows, err := q.MeaningSourcesAfter(ctx, store.MeaningSourcesAfterParams{CursorID: cursorID, RowLimit: int32(limit)}) //nolint:gosec // a batch size
	if err != nil {
		return nil, err
	}
	out := make([]MeaningSource, len(rows))
	for i, r := range rows {
		out[i] = MeaningSource{ID: r.ID, CurrentTitle: deref(r.CurrentTitle)}
	}
	return withWork(ctx, q, out)
}

// MeaningSources returns these candidates (those that exist), in the caller's transaction.
func (m *Module) MeaningSources(ctx context.Context, tx pgx.Tx, ids []string) ([]MeaningSource, error) {
	q := store.New(tx)
	rows, err := q.MeaningSourcesOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]MeaningSource, len(rows))
	for i, r := range rows {
		out[i] = MeaningSource{ID: r.ID, CurrentTitle: deref(r.CurrentTitle)}
	}
	return withWork(ctx, q, out)
}

func withWork(ctx context.Context, q *store.Queries, sources []MeaningSource) ([]MeaningSource, error) {
	if len(sources) == 0 {
		return sources, nil
	}
	at := make(map[string]int, len(sources))
	ids := make([]string, len(sources))
	for i, s := range sources {
		at[s.ID], ids[i] = i, s.ID
	}
	work, err := q.WorkOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, w := range work {
		i := at[w.CandidateID]
		sources[i].Work = append(sources[i].Work, MeaningWork{
			Position: int(w.Position), Title: deref(w.Title), Company: deref(w.Company), StartDate: deref(w.StartDate),
			EndDate: deref(w.EndDate), Detail: deref(w.Description), Current: w.IsCurrent,
		})
	}
	return sources, nil
}
