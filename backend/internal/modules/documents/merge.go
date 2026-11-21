package documents

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// MoveCandidate moves this module's records about candidate fromID to toID,
// inside the transaction merging two duplicate candidates.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	ids := []string{fromID, toID}
	slices.Sort(ids)
	for _, id := range ids {
		if err := q.LockCandidateCVs(ctx, id); err != nil {
			return err
		}
	}
	for _, formatted := range []bool{false, true} {
		var total int
		for _, id := range ids {
			files, err := q.CVsInCategory(ctx, store.CVsInCategoryParams{CandidateID: id, Formatted: formatted})
			if err != nil {
				return err
			}
			total += len(files)
		}
		if total > cvLimit(formatted) {
			category := "non-formatted"
			if formatted {
				category = "formatted"
			}
			return apperr.New(api.ErrorCodeDOCUMENTLIMIT, fmt.Sprintf("These candidates have %d %s CVs together. The merged profile can keep %d.", total, category, cvLimit(formatted)))
		}
	}
	// The Bin keeps references to CV IDs. Move those CVs with the active ones
	// so they do not disappear when the duplicate candidate is deleted.
	return withBinnedDocuments(ctx, tx, func() error {
		_, err := q.MoveDocumentsToCandidate(ctx, store.MoveDocumentsToCandidateParams{FromID: fromID, ToID: toID})
		return err
	})
}
