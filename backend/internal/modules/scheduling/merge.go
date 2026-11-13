package scheduling

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
)

// MoveCandidate moves a merged duplicate's booking links to the candidate
// kept, in the merge's transaction: a link already sent keeps working.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveBookingLinks(ctx, store.MoveBookingLinksParams{FromID: fromID, ToID: toID})
}
