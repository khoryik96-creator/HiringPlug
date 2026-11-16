package app_test

import (
	"context"
	"net/url"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Test packages run in parallel, each with its own database in one
// PostgreSQL, and each sets up the same login roles: they must not trip over
// each other ("tuple concurrently updated").
func TestLoginRolesCanBeSetUpByManyAtOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const role = "hp_role_race_test"
	// A second database in the same PostgreSQL, as another test package has.
	other, err := url.Parse(testutil.Current.AdminURL)
	require.NoError(t, err)
	other.Path = "/postgres"
	otherAdmin, err := pgxpool.New(ctx, other.String())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DROP ROLE IF EXISTS `+role)
		otherAdmin.Close()
	})
	login := "postgres://" + role + ":secret-for-tests@localhost/hiringplug"
	var wg sync.WaitGroup
	errs := make([]error, 16)
	for i := range errs {
		admin := testutil.Current.Admin
		if i%2 == 1 {
			admin = otherAdmin
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = app.EnsureLoginRole(ctx, admin, login, app.GroupApp)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var exists bool
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists))
	assert.True(t, exists)
}

// Companies with the same name made at the same moment each get a careers
// address of their own.
func TestCompaniesWithOneNameCanBeMadeAtOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	name := "Same name for a careers address race " + t.Name()
	ids := make([]string, 8)
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = app.CreateTenant(ctx, testutil.Current.Admin, name)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var slugs int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(DISTINCT careers_slug) FROM tenant WHERE id = ANY($1)`, ids).Scan(&slugs))
	assert.Equal(t, len(ids), slugs)
}
