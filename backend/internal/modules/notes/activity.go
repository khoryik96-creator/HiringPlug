package notes

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes/internal/store"
)

// CallNote is a call logged as a note (a screening call, a reference check or
// a note of a call), for each user's activity in reports.
type CallNote struct {
	ID, Kind, AuthorID string // kind: screening, reference or call
	At                 time.Time
}

// CallsLogged returns the calls logged as notes in [from, to), in the
// caller's transaction (sensitive ones only for those who may read them).
func (m *Module) CallsLogged(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]CallNote, error) {
	rows, err := store.New(tx).CallNotesBetween(ctx, store.CallNotesBetweenParams{FromTime: from, ToTime: to})
	out := make([]CallNote, len(rows))
	for i, r := range rows {
		out[i] = CallNote{ID: r.ID, Kind: r.Kind, AuthorID: r.AuthorID, At: r.CreatedAt}
	}
	return out, err
}
