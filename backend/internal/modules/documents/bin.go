package documents

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
)

// PurgeCandidate deletes a candidate's documents for good (the Bin); after
// commits, their stored files (and PDF copies) are removed.
func (m *Module) PurgeCandidate(ctx context.Context, tx pgx.Tx, candidateID string) ([]string, error) {
	rows, err := store.New(tx).PurgeCandidateDocuments(ctx, candidateID)
	var keys []string
	for _, r := range rows {
		keys = append(keys, r.StorageKey)
		if r.PdfKey != nil {
			keys = append(keys, *r.PdfKey)
		}
	}
	return keys, err
}

// CVTextOf returns a document's candidate, kind (source) and text, in the
// caller's transaction.
func (m *Module) CVTextOf(ctx context.Context, tx pgx.Tx, documentID string) (candidateID, source, text string, err error) {
	row, err := store.New(tx).CVTextOf(ctx, documentID)
	return row.CandidateID, row.Source, row.Text, err
}
