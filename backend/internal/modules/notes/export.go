package notes

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes/internal/store"
)

// Exported is a note as it goes into a copy of a candidate's data.
type Exported struct {
	Kind, Body, AuthorName string
	CreatedAt              time.Time
}

// ForExport returns the notes about a candidate, newest first, as the user
// in tx may see them, and how many sensitive ones they may not see (the
// database hides those).
func (m *Module) ForExport(ctx context.Context, tx pgx.Tx, candidateID string) ([]Exported, int, error) {
	q := store.New(tx)
	rows, err := q.ListNotes(ctx, store.ListNotesParams{SubjectType: "candidate", SubjectID: candidateID})
	if err != nil {
		return nil, 0, err
	}
	hidden, err := q.HiddenSensitiveNotes(ctx, store.HiddenSensitiveNotesParams{SubjectType: "candidate", SubjectID: candidateID})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Exported, len(rows))
	for i, r := range rows {
		out[i] = Exported{Kind: r.Kind, Body: r.Body, AuthorName: r.AuthorName, CreatedAt: r.CreatedAt}
	}
	return out, int(hidden), nil
}
