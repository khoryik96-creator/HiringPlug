package notes

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes/internal/store"
)

// DeleteFor deletes the notes on records deleted for good (the Bin):
// subjectType candidate, contact, company or job.
func (m *Module) DeleteFor(ctx context.Context, tx pgx.Tx, subjectType string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return store.New(tx).DeleteSubjectNotes(ctx, store.DeleteSubjectNotesParams{SubjectType: subjectType, SubjectIds: ids})
}
