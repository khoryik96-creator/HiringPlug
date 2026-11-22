package leads

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/leads/internal/store"
)

// MoveCompany links the leads and lead sources of company fromID to toID,
// inside the transaction merging two duplicate companies.
func (m *Module) MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	if err := q.MoveCompanyLeads(ctx, store.MoveCompanyLeadsParams{FromID: &fromID, ToID: &toID}); err != nil {
		return err
	}
	return q.MoveCompanyLeadSources(ctx, store.MoveCompanyLeadSourcesParams{FromID: &fromID, ToID: &toID})
}
