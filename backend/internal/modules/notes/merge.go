package notes

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes/internal/store"
)

// MoveCandidate moves this module's records about candidate fromID to toID,
// inside the transaction merging two duplicate candidates.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	_, err := q.MoveCandidateNotes(ctx, store.MoveCandidateNotesParams{FromID: fromID, ToID: toID})
	return err
}
