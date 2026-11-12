package backup_test

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// fakeBackup writes a backup folder with parts of the given sizes.
func fakeBackup(t *testing.T, dir, name string, dumpSize, filesSize int, manifest string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "database.dump"), bytes.Repeat([]byte("d"), dumpSize), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "files.tar.gz"), bytes.Repeat([]byte("f"), filesSize), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "manifest.json"), []byte(manifest), 0o600))
}

func TestBackupsAreListedWithTheirDownloadSizeNewestFirst(t *testing.T) {
	dir := t.TempDir()
	fakeBackup(t, dir, "hiringplug-20261101-020000", 1000, 513, `{"createdAt":"2026-11-01T02:00:05Z","files":12}`)
	fakeBackup(t, dir, "hiringplug-20261102-020000", 10, 0, `not json`)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hiringplug-20261103-020000.partial"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "something-else"), 0o700))

	list, err := backup.List(dir)
	require.NoError(t, err)
	require.Len(t, list, 2, "a backup still being written, or another folder, is not one")
	assert.Equal(t, "hiringplug-20261102-020000", list[0].Name)
	assert.Equal(t, time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC), list[0].CreatedAt, "no readable manifest: the time in its name")
	assert.Equal(t, 0, list[0].Files)
	assert.Equal(t, time.Date(2026, 11, 1, 2, 0, 5, 0, time.UTC), list[1].CreatedAt)
	assert.Equal(t, 12, list[1].Files)

	missing, err := backup.List(filepath.Join(dir, "none"))
	require.NoError(t, err)
	assert.Empty(t, missing, "no backups folder yet: no backups")
}

func TestABackupDownloadsAsOneTarOfExactlyItsSize(t *testing.T) {
	dir := t.TempDir()
	name := "hiringplug-20261101-020000"
	fakeBackup(t, dir, name, 1000, 513, `{"files":1}`)
	size, err := backup.Size(dir, name)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, backup.WriteTar(&buf, dir, name))
	assert.Equal(t, size, int64(buf.Len()), "the size shown is the download's size, to the byte")

	tr := tar.NewReader(&buf)
	got := map[string]int{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		got[hdr.Name] = len(body)
	}
	assert.Equal(t, map[string]int{
		name + "/": 0, name + "/database.dump": 1000, name + "/files.tar.gz": 513, name + "/manifest.json": 11,
	}, got, "unpacking it gives the backup's folder back")
}

func TestOnlyBackupsCanBeSentOrDeleted(t *testing.T) {
	dir := t.TempDir()
	fakeBackup(t, dir, "hiringplug-20261101-020000", 1, 1, "{}")
	for _, name := range []string{"../etc", "hiringplug-20261101-020000/../x", "", "hiringplug-2026", "hiringplug-20261109-020000"} {
		assert.ErrorIs(t, backup.WriteTar(io.Discard, dir, name), backup.ErrNotFound, name)
		assert.ErrorIs(t, backup.Delete(dir, name), backup.ErrNotFound, name)
	}
	require.NoError(t, backup.Delete(dir, "hiringplug-20261101-020000"))
	list, err := backup.List(dir)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestWhenTheBackupServiceMakesABackup(t *testing.T) {
	now := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	yesterday, anHourAgo := now.Add(-day), now.Add(-time.Hour)
	tests := []struct {
		name string
		c    backup.Control
		last *time.Time
		want bool
	}{
		{"on, never made", backup.Control{Auto: true}, nil, true},
		{"on, a day since the last", backup.Control{Auto: true}, &yesterday, true},
		{"on, made an hour ago", backup.Control{Auto: true}, &anHourAgo, false},
		{"off: never on its own", backup.Control{Auto: false}, &yesterday, false},
		{"off, but asked for", backup.Control{Auto: false, RequestedAt: &now}, &anHourAgo, true},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, backup.Due(now, tc.c, tc.last, day), tc.name)
	}
}

// The switch and "Back up now" as the backup service sees them: asked for,
// answered by the next backup, made or not.
func TestABackupAskedForIsAnsweredByTheNextOne(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	admin := testutil.Current.Admin
	t.Cleanup(func() { _ = backup.SetAuto(ctx, admin, true) })

	require.NoError(t, backup.SetAuto(ctx, admin, false))
	require.NoError(t, backup.Request(ctx, admin))
	c, err := backup.CheckIn(ctx, admin)
	require.NoError(t, err)
	assert.False(t, c.Auto)
	require.NotNil(t, c.RequestedAt)
	require.NotNil(t, c.ServiceSeenAt, "checking in says the service is running")

	o := backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: t.TempDir(), Keep: 1}
	_, err = backup.Run(ctx, admin, o)
	require.NoError(t, err)
	require.NoError(t, backup.Answered(ctx, admin))
	c, err = backup.ReadControl(ctx, admin)
	require.NoError(t, err)
	assert.Nil(t, c.RequestedAt, "made: no longer asked for")
}
