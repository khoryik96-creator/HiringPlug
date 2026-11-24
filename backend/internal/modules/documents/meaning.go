package documents

// What search by meaning reads of CVs (the meaning module, through internal/app).

import (
	"context"
	"time"

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

// CVKey is which CV is a candidate's newest, and which reading of it (when
// its row last changed), without its text.
type CVKey struct {
	DocumentID string
	Version    time.Time
}

// NewestCVKeys returns the CV NewestCVTexts would give for each candidate,
// without its text (a cheap check of whether it changed).
func (m *Module) NewestCVKeys(ctx context.Context, tx pgx.Tx, candidateIDs []string) (map[string]CVKey, error) {
	rows, err := store.New(tx).NewestCVKeys(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]CVKey, len(rows))
	for _, r := range rows {
		out[r.CandidateID] = CVKey{DocumentID: r.ID, Version: r.UpdatedAt}
	}
	return out, nil
}

// DocumentOwner returns whose a document is and its source (no text), in the caller's transaction.
func (m *Module) DocumentOwner(ctx context.Context, tx pgx.Tx, id string) (candidateID, source string, err error) {
	row, err := store.New(tx).DocumentOwner(ctx, id)
	return row.CandidateID, row.Source, err
}

// DocumentExists says whether a document is still there, in the caller's transaction.
func (m *Module) DocumentExists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	_, err := store.New(tx).DocumentOwner(ctx, id)
	if database.IsNoRows(err) {
		return false, nil
	}
	return err == nil, err
}
