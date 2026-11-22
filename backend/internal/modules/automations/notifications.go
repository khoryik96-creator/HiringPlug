package automations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations/internal/store"
)

// DueTask is an open task of a user's that is due.
type DueTask struct {
	ID, Title     string
	DueOn         time.Time // a day (midnight UTC)
	ApplicationID *string
	CreatedAt     time.Time
}

// DueTasks returns a user's open tasks due on or before day, soonest first
// (at most 50), in the caller's transaction.
func (m *Module) DueTasks(ctx context.Context, tx pgx.Tx, userID string, day time.Time) ([]DueTask, error) {
	rows, err := store.New(tx).DueTasks(ctx, store.DueTasksParams{UserID: userID, Day: day})
	if err != nil {
		return nil, err
	}
	out := make([]DueTask, len(rows))
	for i, r := range rows {
		out[i] = DueTask{ID: r.ID, Title: r.Title, DueOn: r.DueOn, ApplicationID: r.ApplicationID, CreatedAt: r.CreatedAt}
	}
	return out, nil
}
