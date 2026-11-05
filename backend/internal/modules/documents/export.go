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

// CVHit is a candidate whose CV shares words with a text.
type CVHit struct {
	CandidateID string
	Rank        float64 // 0 to 1, for ordering
	Snippet     string  // where the CV has the words, marked [[like this]]
}

// SearchCVs returns the candidates whose CVs (not formatted ones) share the
// most words with text, best first, at most limit, leaving out exclude, in
// the caller's transaction (so only candidates the user may see).
func (m *Module) SearchCVs(ctx context.Context, tx pgx.Tx, text string, exclude []string, limit int) ([]CVHit, error) {
	if exclude == nil {
		exclude = []string{}
	}
	rows, err := store.New(tx).SearchCVs(ctx, store.SearchCVsParams{JobText: text, Exclude: exclude, RowLimit: int32(min(max(limit, 1), 200))}) //nolint:gosec // capped
	out := make([]CVHit, len(rows))
	for i, r := range rows {
		out[i] = CVHit{CandidateID: r.CandidateID, Rank: float64(r.Rank), Snippet: r.Snippet}
	}
	return out, err
}

// WhenTextRead registers what runs when a candidate's CV has been read
// (called by internal/app once every module exists).
func (m *Module) WhenTextRead(hooks ...TextReadHook) { m.service.whenRead = hooks }

// WhenReadDone registers what runs when reading a document has ended.
func (m *Module) WhenReadDone(hooks ...ReadDoneHook) { m.service.whenDone = hooks }

// UseAI lets the AI fill in what the rules leave empty when reading CVs,
// while ai allows it.
func (m *Module) UseAI(ai AIAllowance) { m.service.ai = ai }

// TokensSince returns the AI tokens the company used reading CVs since a
// time (part of its monthly allowance), in the caller's transaction.
func (m *Module) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).AITokensSince(ctx, since)
}
