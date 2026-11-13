package backup_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Main(m)) }

func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"pg_dump", "pg_restore"} {
		_, err := exec.LookPath(tool)
		require.NoError(t, err, "backups need %s (install the PostgreSQL client tools)", tool)
	}
}

func TestABackupHoldsTheDatabaseAndFilesAndRestoresBoth(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	files, dir := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(files, tenant.ID), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(files, tenant.ID, "doc_1"), []byte("%PDF-1.7 cv"), 0o600))

	clock := time.Date(2026, 11, 7, 2, 0, 0, 0, time.UTC)
	o := backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: files, Dir: dir, Keep: 2, Now: func() time.Time { return clock }}
	var made []string
	for range 3 {
		d, err := backup.Run(ctx, testutil.Current.Admin, o)
		require.NoError(t, err)
		made = append(made, filepath.Base(d))
		clock = clock.Add(24 * time.Hour)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var kept []string
	for _, e := range entries {
		kept = append(kept, e.Name())
	}
	assert.Equal(t, made[1:], kept, "only the newest two are kept")
	for _, f := range []string{"database.dump", "manifest.json", filepath.Join("files", tenant.ID, "doc_1")} {
		info, err := os.Stat(filepath.Join(dir, made[2], f))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "%s: only the server's own user may read it", f)
	}
	// The file did not change between the two nights: both backups hold it,
	// once on the disk.
	older, err := os.Stat(filepath.Join(dir, made[1], "files", tenant.ID, "doc_1"))
	require.NoError(t, err)
	newer, err := os.Stat(filepath.Join(dir, made[2], "files", tenant.ID, "doc_1"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(older, newer), "an unchanged file is shared with the backup before, not copied again")
	// What Settings shows is the download's size to the byte, files included.
	size, err := backup.Size(dir, made[2])
	require.NoError(t, err)
	var sent countingWriter
	require.NoError(t, backup.WriteTar(&sent, dir, made[2]))
	assert.Equal(t, size, sent.n)
	info, err := backup.Stat(dir, made[2])
	require.NoError(t, err)
	assert.Equal(t, 1, info.Files)
	assert.Less(t, info.DiskBytes, info.SizeBytes, "it added less to the disk than its download: its file is shared")

	last, ok, err := backup.LastRun(ctx, testutil.Current.App)
	require.NoError(t, err)
	require.True(t, ok, "the API can see the last backup")
	assert.Equal(t, made[2], last.Name)
	assert.Equal(t, "done", last.Status)
	require.NotNil(t, last.SizeBytes)
	assert.Positive(t, *last.SizeBytes)
	_, err = testutil.Current.App.Exec(ctx, `DELETE FROM backup_run`)
	require.Error(t, err, "only hp backup writes the record")

	// Restored into an empty database and files folder.
	target := freshDatabase(t)
	restored := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(restored, "stale"), []byte("x"), 0o600))
	require.NoError(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: restored, Dir: dir}, made[2]))
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM tenant WHERE id = $1`, tenant.ID).Scan(&n))
	assert.Equal(t, 1, n)
	data, err := os.ReadFile(filepath.Join(restored, tenant.ID, "doc_1"))
	require.NoError(t, err)
	assert.Equal(t, "%PDF-1.7 cv", string(data))
	_, err = os.Stat(filepath.Join(restored, "stale"))
	assert.True(t, os.IsNotExist(err), "the files are the backup's, nothing else")
}

func TestAFailedBackupIsRecordedAndLeavesNoFolder(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	dir := t.TempDir()
	o := backup.Options{AdminURL: "postgres://nobody:secret-pw@127.0.0.1:1/none?sslmode=disable", FilesDir: t.TempDir(), Dir: dir, Keep: 3,
		Now: func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }}
	_, err := backup.Run(ctx, testutil.Current.Admin, o)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-pw", "no password in messages")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
	last, ok, err := backup.LastRun(ctx, testutil.Current.App)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "failed", last.Status)
	require.NotNil(t, last.Error)
}

func TestRestoreRefusesFilesOutsideTheFilesFolder(t *testing.T) {
	dir := t.TempDir()
	name := "hiringplug-20260101-000000"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o700))
	for _, f := range []string{"database.dump", "manifest.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, f), nil, 0o600))
	}
	f, err := os.Create(filepath.Join(dir, name, "files.tar.gz"))
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())

	assert.ErrorContains(t, backup.Restore(context.Background(), backup.Options{Dir: dir}, "../etc"), "not a backup name")
	// The database part fails first here (an empty dump), so the archive is checked on its own.
	err = backup.Untar(filepath.Join(dir, name, "files.tar.gz"), t.TempDir())
	assert.ErrorContains(t, err, "outside the files folder")
}

func freshDatabase(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	u, err := url.Parse(testutil.Current.AdminURL)
	require.NoError(t, err)
	name := testutil.Current.Name + "_restore"
	_, err = testutil.Current.Admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	u.Path = "/" + strings.TrimPrefix(name, "/")
	return u.String()
}

func TestTheToolsMatchTheDatabasesVersion(t *testing.T) {
	p, err := backup.FindTool("pg_dump", 16, "/usr/bin", "17.2", 16, 17)
	require.NoError(t, err)
	assert.Equal(t, "/usr/lib/postgresql/16/bin/pg_dump", p, "the server's own version first")
	p, err = backup.FindTool("pg_restore", 16, "/usr/bin", "16.4")
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/pg_restore", p)
	_, err = backup.FindTool("pg_dump", 16, "/usr/bin", "17.2")
	assert.ErrorContains(t, err, "install the PostgreSQL 16 client tools", "a newer pg_dump's backup could not be put back")
	_, err = backup.FindTool("pg_dump", 16, "", "")
	assert.ErrorContains(t, err, "not installed")
}

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }
