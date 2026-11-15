package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
)

// folderUsers: a folder is shared only with active users of the same agency.
type folderUsers struct{ identity *identity.Module }

func (u folderUsers) Active(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	_, ok, err := u.identity.PrincipalFor(ctx, tx, userID)
	return ok, err
}
