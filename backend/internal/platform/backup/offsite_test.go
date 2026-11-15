package backup_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func requireRclone(t *testing.T) {
	t.Helper()
	requireTools(t)
	_, err := exec.LookPath("rclone")
	require.NoError(t, err, "off-site copies need rclone (apt-get install rclone)")
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// A folder on another disk stands in for the storage account (an S3 bucket
// works the same way through rclone).
func TestEachBackupIsCopiedOffTheServerAndComesBackWhole(t *testing.T) {
	requireRclone(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	files, dir, away := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(files, tenant.ID), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(files, tenant.ID, "doc_1"), []byte("%PDF-1.7 cv"), 0o600))

	clock := time.Date(2026, 11, 8, 2, 0, 0, 0, time.UTC)
	o := backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: files, Dir: dir, Keep: 2,
		Now: func() time.Time { return clock }, Offsite: backup.Offsite{Remote: away}}
	var made []string
	for i := range 3 {
		if i == 2 { // a CV added on the third night
			require.NoError(t, os.WriteFile(filepath.Join(files, tenant.ID, "doc_2"), []byte("%PDF-1.7 new cv"), 0o600))
		}
		d, err := backup.Run(ctx, testutil.Current.Admin, o)
		require.NoError(t, err)
		made = append(made, filepath.Base(d))
		clock = clock.Add(24 * time.Hour)
	}
	assert.Equal(t, append([]string{"files"}, made[1:]...), names(t, away), "the backups kept here, and the files as of the newest")
	for _, f := range []string{"database.dump", "manifest.json"} {
		_, err := os.Stat(filepath.Join(away, made[2], f))
		require.NoError(t, err, f)
	}
	assert.Equal(t, []string{"doc_1", "doc_2"}, names(t, filepath.Join(away, "files", tenant.ID)))
	last, ok, err := backup.LastRun(ctx, testutil.Current.App)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, last.OffsiteStatus)
	assert.Equal(t, "done", *last.OffsiteStatus)
	assert.NotNil(t, last.OffsiteAt)

	// The server is lost: a new one fetches the newest backup and puts it back.
	fresh := t.TempDir()
	got, err := backup.Fetch(ctx, o.Offsite, fresh, "latest")
	require.NoError(t, err)
	assert.Equal(t, made[2], got)
	target, restored := freshDatabase(t), t.TempDir()
	require.NoError(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: restored, Dir: fresh}, got))
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM tenant WHERE id = $1`, tenant.ID).Scan(&n))
	assert.Equal(t, 1, n)
	data, err := os.ReadFile(filepath.Join(restored, tenant.ID, "doc_2"))
	require.NoError(t, err)
	assert.Equal(t, "%PDF-1.7 new cv", string(data))

	_, err = backup.Fetch(ctx, o.Offsite, fresh, got)
	assert.ErrorContains(t, err, "already here")
	_, err = backup.Fetch(ctx, o.Offsite, t.TempDir(), "hiringplug-20200101-000000")
	assert.ErrorContains(t, err, "no backup")
}

func TestAFailedOffsiteCopyIsRecordedAndTheBackupKept(t *testing.T) {
	requireRclone(t)
	ctx := context.Background()
	dir := t.TempDir()
	blocked := filepath.Join(t.TempDir(), "not-a-folder")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600)) // a file where the storage should be
	o := backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: dir, Keep: 3,
		Now: func() time.Time { return time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC) }, Offsite: backup.Offsite{Remote: blocked}}
	_, err := backup.Run(ctx, testutil.Current.Admin, o)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copy off the server failed")
	assert.Equal(t, []string{"hiringplug-20261120-020000"}, names(t, dir), "the backup itself is made")
	last, ok, err := backup.LastRun(ctx, testutil.Current.App)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "done", last.Status)
	require.NotNil(t, last.OffsiteStatus)
	assert.Equal(t, "failed", *last.OffsiteStatus)
	require.NotNil(t, last.OffsiteError)

	// Without off-site storage, the record says so.
	o.Offsite, o.Now = backup.Offsite{}, func() time.Time { return time.Date(2026, 11, 21, 2, 0, 0, 0, time.UTC) }
	_, err = backup.Run(ctx, testutil.Current.Admin, o)
	require.NoError(t, err)
	last, _, err = backup.LastRun(ctx, testutil.Current.App)
	require.NoError(t, err)
	require.NotNil(t, last.OffsiteStatus)
	assert.Equal(t, "off", *last.OffsiteStatus)
}
