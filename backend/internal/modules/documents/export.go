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

// SearchTerm is a word or phrase to look for in CVs, with how much it counts.
type SearchTerm struct {
	Text   string
	Weight float32
}

// SearchCVs returns the candidates whose CVs (not formatted ones) best match
// the weighted terms (each weighed again by how rare it is in the company's
// CVs), best first, at most limit, leaving out exclude, in the caller's
// transaction (so only candidates the user may see).
func (m *Module) SearchCVs(ctx context.Context, tx pgx.Tx, terms []SearchTerm, exclude []string, limit int) ([]CVHit, error) {
	if exclude == nil {
		exclude = []string{}
	}
	words, weights := make([]string, len(terms)), make([]float32, len(terms))
	for i, t := range terms {
		words[i], weights[i] = t.Text, t.Weight
	}
	rows, err := store.New(tx).SearchCVs(ctx, store.SearchCVsParams{Terms: words, Weights: weights, Exclude: exclude, RowLimit: int32(min(max(limit, 1), 200))}) //nolint:gosec // capped
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

// WhenCVBinned registers what runs when a candidate's CV is moved to the Bin or back.
func (m *Module) WhenCVBinned(hooks ...CVBinnedHook) { m.service.whenBinned = hooks }

// UseAI lets the AI fill in what the rules leave empty when reading CVs,
// while ai allows it.
func (m *Module) UseAI(ai AIAllowance) { m.service.ai = ai }

// TokensSince returns the AI tokens the company used reading CVs since a
// time (part of its monthly allowance), in the caller's transaction.
func (m *Module) TokensSince(ctx context.Context, tx pgx.Tx, since time.Time) (int64, error) {
	return store.New(tx).AITokensSince(ctx, since)
}
