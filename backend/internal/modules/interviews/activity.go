package interviews

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
)

// Arranged is an interview someone scheduled, and when they did.
type Arranged struct {
	By string
	At time.Time
}

// ArrangedBetween returns the interviews scheduled in [from, to) (whatever
// became of them), in the caller's transaction: each user's activity.
func (m *Module) ArrangedBetween(ctx context.Context, tx pgx.Tx, from, to time.Time) ([]Arranged, error) {
	rows, err := store.New(tx).ArrangedBetween(ctx, store.ArrangedBetweenParams{FromTime: from, ToTime: to})
	out := make([]Arranged, len(rows))
	for i, r := range rows {
		out[i] = Arranged{By: r.CreatedBy, At: r.CreatedAt}
	}
	return out, err
}
