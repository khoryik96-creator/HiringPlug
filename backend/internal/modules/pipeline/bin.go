package pipeline

// The Bin (bin module): the applications of a binned candidate or job are
// hidden with them (application.binned), and deleted with them for good.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline/internal/store"
)

// Linked is an application, by candidate and job.
type Linked struct{ ID, CandidateID, JobID string }

// ApplicationsOf returns the applications of these candidates or jobs (hidden ones too, in the bin's transaction).
func (m *Module) ApplicationsOf(ctx context.Context, tx pgx.Tx, candidateIDs, jobIDs []string) ([]Linked, error) {
	rows, err := store.New(tx).ApplicationsOf(ctx, store.ApplicationsOfParams{CandidateIds: nonNil(candidateIDs), JobIds: nonNil(jobIDs)})
	out := make([]Linked, len(rows))
	for i, r := range rows {
		out[i] = Linked{ID: r.ID, CandidateID: r.CandidateID, JobID: r.JobOrderID}
	}
	return out, err
}

// SetBinned hides (or shows again) applications.
func (m *Module) SetBinned(ctx context.Context, tx pgx.Tx, ids []string, binned bool) error {
	return store.New(tx).SetApplicationsBinned(ctx, store.SetApplicationsBinnedParams{Ids: nonNil(ids), Binned: binned})
}

// Purge deletes hidden applications for good (interviews, offers and moves with them).
func (m *Module) Purge(ctx context.Context, tx pgx.Tx, ids []string) error {
	return store.New(tx).PurgeApplications(ctx, nonNil(ids))
}

func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
