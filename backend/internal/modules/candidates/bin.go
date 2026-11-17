package candidates

// The Bin (bin module): a binned candidate is hidden by the database until
// restored, then deleted for good. Called inside the bin module's
// transaction, which sees binned records.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ErrEmailTaken: a candidate in the Bin cannot be restored while another has its email.
var ErrEmailTaken = errors.New("another candidate has this email now")

// Label is a candidate's name (not found when binned or not theirs to see).
func (m *Module) Label(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	label, err := store.New(tx).CandidateLabel(ctx, id)
	if database.IsNoRows(err) {
		return "", apperr.NotFound("candidate")
	}
	return label, err
}

// Bin hides a candidate and returns their name.
func (m *Module) Bin(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	label, err := store.New(tx).BinCandidate(ctx, id)
	if database.IsNoRows(err) {
		return "", apperr.NotFound("candidate")
	}
	return label, err
}

// Restore shows a binned candidate again.
func (m *Module) Restore(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := store.New(tx).RestoreCandidate(ctx, id)
	if database.IsUniqueViolation(err, emailUniqueIndex) {
		return ErrEmailTaken
	}
	return err
}

// Binned returns which of ids are in the Bin.
func (m *Module) Binned(ctx context.Context, tx pgx.Tx, ids []string) ([]string, error) {
	return store.New(tx).BinnedCandidates(ctx, ids)
}

// Purge deletes a binned candidate for good (with their salary, history,
// emails and consent; CVs, jobs and notes are removed by their modules first).
func (m *Module) Purge(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := store.New(tx).PurgeCandidate(ctx, id)
	return err
}
