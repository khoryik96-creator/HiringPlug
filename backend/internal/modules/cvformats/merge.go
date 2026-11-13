package cvformats

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/cvformats/internal/store"
)

// MoveCandidate moves a merged duplicate's formatting runs to the candidate
// kept, in the merge's transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveStudioCvs(ctx, store.MoveStudioCvsParams{FromID: fromID, ToID: toID})
}
