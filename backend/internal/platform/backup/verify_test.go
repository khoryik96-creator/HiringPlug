package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// From the owner's post-migration report (P1): a backup is checked as it is
// made, and on request (hp backup verify), without restoring it: one whose
// database copy is cut short, or which lost a file, is found now, not on the
// day it is needed.
func TestABackupIsCheckedToReadBack(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	files, dir := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(files, tenant.ID), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(files, tenant.ID, "doc_1"), []byte("%PDF-1.7 cv"), 0o600))
	clock := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	o := backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: files, Dir: dir, Keep: 5, Now: func() time.Time { return clock }}
	made, err := backup.Run(ctx, testutil.Current.Admin, o)
	require.NoError(t, err, "checked as it is made")
	name := filepath.Base(made)

	got, err := backup.Verify(ctx, o, name)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Files)
	assert.Greater(t, got.Tables, 10, "every table with data is in the copy")

	// Cut short on its way off the server, or on a failing disk.
	dump := filepath.Join(made, "database.dump")
	info, err := os.Stat(dump)
	require.NoError(t, err)
	require.NoError(t, os.Truncate(dump, info.Size()/2))
	_, err = backup.Verify(ctx, o, name)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "its database copy cannot be read")

	// A file lost.
	clock = clock.Add(24 * time.Hour)
	second, err := backup.Run(ctx, testutil.Current.Admin, o)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(second, "files", tenant.ID, "doc_1")))
	_, err = backup.Verify(ctx, o, filepath.Base(second))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lists 1 files but holds 0")

	_, err = backup.Verify(ctx, o, "not-a-backup")
	require.Error(t, err)
}
