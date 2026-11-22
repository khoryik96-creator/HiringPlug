package placements

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/placements/internal/store"
)

// MoveContact addresses the invoices sent to contact fromID to toID, inside
// the transaction merging two duplicate contacts.
func (m *Module) MoveContact(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	_, err := store.New(tx).MoveInvoicesToContact(ctx, store.MoveInvoicesToContactParams{FromID: &fromID, ToID: &toID})
	return err
}
