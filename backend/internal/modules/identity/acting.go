package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// PrincipalFor is a user as the app acts for them (an automation sending
// from their mailbox), in the caller's transaction: their company's
// permissions for their role. False when there is no such active user.
func (m *Module) PrincipalFor(ctx context.Context, tx pgx.Tx, userID string) (authz.Principal, bool, error) {
	q := store.New(tx)
	u, err := q.UserAuth(ctx, userID)
	if database.IsNoRows(err) {
		return authz.Principal{}, false, nil
	}
	if err != nil || !u.Active || u.Invited {
		return authz.Principal{}, false, err
	}
	p, err := principalOf(ctx, q, u)
	return p, err == nil, err
}
