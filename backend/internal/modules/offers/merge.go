package offers

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/offers/internal/store"
)

// OnApplications reports which of some applications have an offer, in the
// caller's transaction (merging candidates never deletes those).
func (m *Module) OnApplications(ctx context.Context, tx pgx.Tx, applicationIDs []string) (map[string]bool, error) {
	ids, err := store.New(tx).ApplicationsWithOffers(ctx, applicationIDs)
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, err
}
