package candidates

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
)

// CallMade is a screening call as reports count it: who made it, the day
// it was, and the note written with it (nil when none).
type CallMade struct {
	CalledBy string
	CalledOn time.Time
	NoteID   *string
}

// ScreeningCallsBetween returns the screening calls made between two days
// (inclusive), in the caller's transaction.
func (m *Module) ScreeningCallsBetween(ctx context.Context, tx pgx.Tx, fromDay, toDay time.Time) ([]CallMade, error) {
	rows, err := store.New(tx).ScreeningCallsBetween(ctx, store.ScreeningCallsBetweenParams{
		FromDay: pgtype.Date{Time: fromDay, Valid: true}, ToDay: pgtype.Date{Time: toDay, Valid: true},
	})
	out := make([]CallMade, len(rows))
	for i, r := range rows {
		out[i] = CallMade{CalledBy: r.CalledBy, CalledOn: r.CalledOn.Time, NoteID: r.NoteID}
	}
	return out, err
}

// ScreeningCallNotes returns those of some notes that were written with a
// screening call, in the caller's transaction.
func (m *Module) ScreeningCallNotes(ctx context.Context, tx pgx.Tx, noteIDs []string) ([]string, error) {
	return store.New(tx).ScreeningCallNoteIDs(ctx, noteIDs)
}
