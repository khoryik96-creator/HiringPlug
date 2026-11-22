package notes

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/notes/internal/store"
)

// MoveCandidate moves this module's records about candidate fromID to toID,
// inside the transaction merging two duplicate candidates.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	_, err := q.MoveCandidateNotes(ctx, store.MoveCandidateNotesParams{FromID: fromID, ToID: toID})
	return err
}

// MoveCompany moves the notes about company fromID to toID (sensitive ones
// too), inside the transaction merging two duplicate companies.
func (m *Module) MoveCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveSubjectNotes(ctx, store.MoveSubjectNotesParams{SubjectType: "company", FromID: fromID, ToID: toID})
}

// MoveContact moves the notes about contact fromID to toID, inside the
// transaction merging two duplicate contacts.
func (m *Module) MoveContact(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	return store.New(tx).MoveSubjectNotes(ctx, store.MoveSubjectNotesParams{SubjectType: "contact", FromID: fromID, ToID: toID})
}
