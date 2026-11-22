package offers

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/offers/internal/store"
)

// Pending is an offer waiting for approval.
type Pending struct {
	ID, ApplicationID string
	At                time.Time // when it last changed (was sent for approval)
}

// AwaitingApprovalBy returns the offers waiting for approval that the user
// did not ask for themselves (no one approves their own), newest first (at
// most 50), in the caller's transaction.
func (m *Module) AwaitingApprovalBy(ctx context.Context, tx pgx.Tx, userID string) ([]Pending, error) {
	rows, err := store.New(tx).AwaitingApproval(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Pending, len(rows))
	for i, r := range rows {
		out[i] = Pending{ID: r.ID, ApplicationID: r.ApplicationID, At: r.UpdatedAt}
	}
	return out, nil
}
