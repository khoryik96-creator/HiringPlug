package app_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Setting HP_DB_ADMIN_PASSWORD after the database was created (Docker fixes
// it on the first start) makes bootstrap fail: it says why and what to do.
func TestAWrongAdminPasswordIsExplained(t *testing.T) {
	u, err := url.Parse(testutil.Current.AdminURL)
	require.NoError(t, err)
	u.User = url.UserPassword(u.User.Username(), "not-the-password-it-was-created-with")
	_, err = database.Connect(context.Background(), u.String())
	require.Error(t, err)
	explained := app.ExplainAdminLogin(err)
	assert.Contains(t, explained.Error(), "HP_DB_ADMIN_PASSWORD")
	assert.Contains(t, explained.Error(), "first created")
	assert.ErrorIs(t, explained, err, "the cause is kept")
	other := errors.New("connection refused")
	assert.Equal(t, other, app.ExplainAdminLogin(other), "other problems are left as they are")
	assert.NoError(t, app.ExplainAdminLogin(nil))
}
