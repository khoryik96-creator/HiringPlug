package intake

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/intake/internal/store"
)

// MoveCandidate points the import files that made a merged duplicate to the
// candidate kept, in the merge's transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveImportItems(ctx, store.MoveImportItemsParams{FromID: fromID, ToID: toID})
}
