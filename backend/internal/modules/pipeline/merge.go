package pipeline

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
)

// MoveCandidate moves this module's records about candidate fromID to toID,
// inside the transaction merging two duplicate candidates.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	// Both on the same job: the kept candidate's application stays.
	if _, err := q.DropOverlappingApplications(ctx, store.DropOverlappingApplicationsParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	_, err := q.MoveApplicationsToCandidate(ctx, store.MoveApplicationsToCandidateParams{FromID: fromID, ToID: toID})
	return err
}
