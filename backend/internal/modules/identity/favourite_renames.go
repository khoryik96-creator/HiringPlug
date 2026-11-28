package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
)

// RenameFavourite renames a word in one kind of everyone's favourites
// (itSkill, certification or specialization: a word renamed in the company's
// list), in the caller's transaction, and says how many people's changed.
// For someone who manages Settings (the database checks).
func (m *Module) RenameFavourite(ctx context.Context, tx pgx.Tx, kind, from, to string) (int, error) {
	n, err := store.New(tx).RenameFavourite(ctx, store.RenameFavouriteParams{Kind: kind, FromWord: from, ToWord: to})
	return int(n), err
}
