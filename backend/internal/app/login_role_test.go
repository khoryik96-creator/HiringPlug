package app_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Test packages run in parallel and each sets up the same login roles: they
// must not trip over each other ("tuple concurrently updated").
func TestLoginRolesCanBeSetUpByManyAtOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	url := "postgres://hp_role_race_test:secret-for-tests@localhost/hiringplug"
	var wg sync.WaitGroup
	errs := make([]error, 12)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = app.EnsureLoginRole(ctx, testutil.Current.Admin, url, app.GroupApp)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var exists bool
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hp_role_race_test')`).Scan(&exists))
	assert.True(t, exists)
}
