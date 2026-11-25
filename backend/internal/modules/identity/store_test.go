package identity_test

// The identity module's SQL, against real PostgreSQL (the module's rules
// are tested without a database in *_domain_test.go, and through the API in
// internal/app).

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

// A new password never moves an end of the person's sessions back: one
// made later than the one it was given (by another change at about the
// same time) stays.
func TestANewPasswordNeverMovesAnEndOfSessionsBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	userID := tenant.Users[api.RoleRecruiter].ID
	var later time.Time
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx,
		`UPDATE app_user SET tokens_valid_after = date_trunc('second', now()) + interval '1 hour' WHERE id = $1 RETURNING tokens_valid_after`, userID).Scan(&later))
	hash, earlier := "a-new-hash", later.Add(-time.Hour)
	require.NoError(t, database.InTx(ctx, testutil.Current.App, authz.System(tenant.ID), func(tx pgx.Tx) error {
		return store.New(tx).SetPassword(ctx, store.SetPasswordParams{ID: userID, PasswordHash: &hash, ValidAfter: &earlier})
	}))
	var after time.Time
	var stored string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT tokens_valid_after, password_hash FROM app_user WHERE id = $1`, userID).Scan(&after, &stored))
	assert.True(t, after.Equal(later), "kept %s, got %s", later, after)
	assert.Equal(t, hash, stored)
}
