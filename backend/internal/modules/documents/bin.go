package documents

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
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

// NewestCVOf is the candidate's newest CV they sent of some content types
// (their CV first): "" when they have none.
func (m *Module) NewestCVOf(ctx context.Context, tx pgx.Tx, candidateID string, contentTypes []string) (string, error) {
	id, err := store.New(tx).NewestCVOfTypes(ctx, store.NewestCVOfTypesParams{CandidateID: candidateID, ContentTypes: contentTypes})
	if database.IsNoRows(err) {
		return "", nil
	}
	return id, err
}

// NewestCV is the candidate's newest CV (not one the app made, not in the
// bin): "" when they have none.
func (m *Module) NewestCV(ctx context.Context, tx pgx.Tx, candidateID string) (string, error) {
	id, err := store.New(tx).NewestCVID(ctx, candidateID)
	if database.IsNoRows(err) {
		return "", nil
	}
	return id, err
}
