package aipaid_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/aipaid"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

// Review of 1.47.0: with its row gone, the switch read as an error in Go
// (Settings > System failed) while the AI worker read it as not paused.
// Both read it as not paused now, and changing it says the row is missing.
func TestASwitchWithoutItsRowReadsAsNotPaused(t *testing.T) {
	ctx := context.Background()
	admin := testutil.Current.Admin
	_, err := admin.Exec(ctx, `DELETE FROM ai_paid_control`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `INSERT INTO ai_paid_control DEFAULT VALUES ON CONFLICT (id) DO NOTHING`)
	})
	sw, err := aipaid.Read(ctx, testutil.Current.App)
	require.NoError(t, err)
	assert.False(t, sw.Paused)
	paused, err := aipaid.Paused(ctx, testutil.Current.App)
	require.NoError(t, err)
	assert.Equal(t, paused, sw.Paused, "as the AI worker reads it")
	assert.ErrorIs(t, aipaid.Set(ctx, testutil.Current.App, true, "", "usr_x"), aipaid.ErrNoSwitch)

	_, err = admin.Exec(ctx, `INSERT INTO ai_paid_control DEFAULT VALUES`)
	require.NoError(t, err)
	require.NoError(t, aipaid.Set(ctx, testutil.Current.App, true, "Testing.", "usr_x"))
	sw, err = aipaid.Read(ctx, testutil.Current.App)
	require.NoError(t, err)
	assert.True(t, sw.Paused)
	assert.Equal(t, "Testing.", *sw.Reason)
}
