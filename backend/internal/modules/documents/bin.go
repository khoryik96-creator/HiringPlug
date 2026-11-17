package documents

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// PurgeCandidate deletes a candidate's documents for good (the Bin); after
// commits, their stored files (and PDF copies) are removed.
func (m *Module) PurgeCandidate(ctx context.Context, tx pgx.Tx, candidateID string) (after func(), err error) {
	rows, err := store.New(tx).PurgeCandidateDocuments(ctx, candidateID)
	return func() {
		for _, r := range rows {
			keys := []string{r.StorageKey}
			if r.PdfKey != nil {
				keys = append(keys, *r.PdfKey)
			}
			for _, k := range keys {
				if err := m.service.files.Delete(k); err != nil {
					logx.From(ctx).Warn("a deleted candidate's file could not be removed", "error", err)
				}
			}
		}
	}, err
}
