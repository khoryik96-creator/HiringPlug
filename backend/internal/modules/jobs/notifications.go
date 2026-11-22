package jobs

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
)

// OwnedBy returns the jobs a recruiter owns (the most recent 1,000), in the
// caller's transaction (notifications: new applicants on them).
func (m *Module) OwnedBy(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	return store.New(tx).JobOrderIDsOwnedBy(ctx, &userID)
}
