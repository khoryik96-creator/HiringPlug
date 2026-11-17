package jobs

// The Bin (bin module): a company's jobs go to the Bin with it (hidden by the
// database), or move to another company. Called inside the bin module's
// transaction, which sees binned records.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// Bin hides jobs.
func (m *Module) Bin(ctx context.Context, tx pgx.Tx, ids []string) error {
	return store.New(tx).BinJobs(ctx, ids)
}

// Restore shows binned jobs again.
func (m *Module) Restore(ctx context.Context, tx pgx.Tx, ids []string) error {
	return store.New(tx).RestoreJobs(ctx, ids)
}

// Binned returns which of ids are in the Bin.
func (m *Module) Binned(ctx context.Context, tx pgx.Tx, ids []string) ([]string, error) {
	return store.New(tx).BinnedJobs(ctx, ids)
}

// MoveToCompany moves a company's jobs to another company.
func (m *Module) MoveToCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	_, err := store.New(tx).MoveJobsToCompany(ctx, store.MoveJobsToCompanyParams{FromID: fromID, ToID: toID})
	return err
}

// ClearContacts takes contacts deleted for good off the jobs they were the contact of.
func (m *Module) ClearContacts(ctx context.Context, tx pgx.Tx, contactIDs []string) error {
	return store.New(tx).ClearJobContacts(ctx, contactIDs)
}

// Purge deletes binned jobs for good (their salary, ads and files with them;
// their applications are removed by the pipeline first); after commits,
// their stored files are removed.
func (m *Module) Purge(ctx context.Context, tx pgx.Tx, ids []string) (after func(), err error) {
	keys, err := store.New(tx).PurgeJobs(ctx, ids)
	return func() {
		for _, k := range keys {
			if err := m.service.files.Delete(k); err != nil {
				logx.From(ctx).Warn("a deleted job's file could not be removed", "error", err)
			}
		}
	}, err
}
