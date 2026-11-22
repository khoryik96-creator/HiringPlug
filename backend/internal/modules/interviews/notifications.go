package interviews

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
)

// Upcoming is a scheduled interview of a user's.
type Upcoming struct {
	ID, ApplicationID, Title string
	StartsAt, CreatedAt      time.Time
}

// UpcomingFor returns the scheduled interviews the user interviews on or
// arranged that start in [from, to), soonest first (at most 50), in the
// caller's transaction.
func (m *Module) UpcomingFor(ctx context.Context, tx pgx.Tx, userID string, from, to time.Time) ([]Upcoming, error) {
	rows, err := store.New(tx).UpcomingFor(ctx, store.UpcomingForParams{UserID: userID, FromTime: from, ToTime: to})
	if err != nil {
		return nil, err
	}
	out := make([]Upcoming, len(rows))
	for i, r := range rows {
		out[i] = Upcoming{ID: r.ID, ApplicationID: r.ApplicationID, Title: r.Title, StartsAt: r.StartsAt, CreatedAt: r.CreatedAt}
	}
	return out, nil
}
