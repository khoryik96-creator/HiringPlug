package jobs

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
)

// MoveCompany moves the jobs of company fromID to toID, inside the
// transaction merging two duplicate companies.
func (m *Module) MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	_, err := store.New(tx).MoveCompanyJobs(ctx, store.MoveCompanyJobsParams{FromID: fromID, ToID: toID})
	return err
}

// MoveContact makes contact toID the contact of the jobs that had fromID,
// inside the transaction merging two duplicate contacts.
func (m *Module) MoveContact(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	_, err := store.New(tx).MoveContactJobs(ctx, store.MoveContactJobsParams{FromID: &fromID, ToID: &toID})
	return err
}
