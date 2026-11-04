package documents

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
)

// Exported is a document as it goes into a copy of a candidate's data: what
// it is and the text read from it (the file itself stays in the app).
type Exported struct {
	FileName, Source, Status string
	Text                     *string
	UploadedAt               time.Time
}

// ForExport returns a candidate's documents with their text, newest first,
// in the caller's transaction.
func (m *Module) ForExport(ctx context.Context, tx pgx.Tx, candidateID string) ([]Exported, error) {
	rows, err := store.New(tx).ListDocumentTexts(ctx, candidateID)
	if err != nil {
		return nil, err
	}
	out := make([]Exported, len(rows))
	for i, r := range rows {
		out[i] = Exported{FileName: r.FileName, Source: r.Source, Status: r.Status, Text: r.ExtractedText, UploadedAt: r.CreatedAt}
	}
	return out, nil
}
