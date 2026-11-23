package documents

// What search by meaning reads of CVs (the meaning module, through internal/app).

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// CVText is a candidate's newest CV they sent, with its text.
type CVText struct {
	DocumentID, Text string
}

// NewestCVTexts returns each candidate's newest CV they sent with text (their
// CV first, then a LinkedIn or JobStreet one; never one the app made or one
// in the bin), in the caller's transaction.
func (m *Module) NewestCVTexts(ctx context.Context, tx pgx.Tx, candidateIDs []string) (map[string]CVText, error) {
	rows, err := store.New(tx).NewestCVTexts(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]CVText, len(rows))
	for _, r := range rows {
		out[r.CandidateID] = CVText{DocumentID: r.ID, Text: r.Text}
	}
	return out, nil
}

// DocumentExists says whether a document is still there, in the caller's transaction.
func (m *Module) DocumentExists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	_, err := store.New(tx).CVTextOf(ctx, id)
	if database.IsNoRows(err) {
		return false, nil
	}
	return err == nil, err
}
