package automations

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/automations/internal/store"
)

// MoveCompany makes the automations about company fromID about toID,
// inside the transaction merging two duplicate companies.
func (m *Module) MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	_, err := store.New(tx).MoveCompanyAutomations(ctx, store.MoveCompanyAutomationsParams{FromID: &fromID, ToID: &toID})
	return err
}
