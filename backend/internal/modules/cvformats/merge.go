package cvformats

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
)

// MoveCandidate moves a merged duplicate's formatting runs (and what is
// known of how their CVs were made) to the candidate kept, in the merge's
// transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	if err := q.MoveStudioCvs(ctx, store.MoveStudioCvsParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	return q.MoveFormattedCvs(ctx, store.MoveFormattedCvsParams{FromID: fromID, ToID: toID})
}
