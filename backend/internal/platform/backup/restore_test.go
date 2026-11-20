package backup_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestFailedRestorePreservesDatabaseAndFiles(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	testutil.Current.NewTenant(t)
	source, dest, root := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "old.pdf"), []byte("synthetic archived file"), 0o600))
	made, err := backup.Run(ctx, testutil.Current.Admin, backup.Options{
		AdminURL: testutil.Current.AdminURL, FilesDir: source, Dir: root, Keep: 1,
	})
	require.NoError(t, err)
	valid := filepath.Join(t.TempDir(), "valid-files")
	require.NoError(t, os.Rename(filepath.Join(made, "files"), valid))
	require.NoError(t, os.WriteFile(filepath.Join(made, "files.tar.gz"), []byte("invalid archive fixture"), 0o600))
	marker := filepath.Join(dest, "current.pdf")
	require.NoError(t, os.WriteFile(marker, []byte("synthetic existing file"), 0o600))
	target := freshDatabase(t)
	err = backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: root}, filepath.Base(made))
	require.Error(t, err)
	require.FileExists(t, marker)
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var absent bool
	require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass('public.tenant') IS NULL").Scan(&absent))
	require.True(t, absent)
	// A valid tar with a damaged gzip trailer also fails before installation.
	var packed bytes.Buffer
	gz := gzip.NewWriter(&packed)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "old.pdf", Size: 1, Mode: 0o600}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	raw := packed.Bytes()
	raw[len(raw)-8] ^= 1
	require.NoError(t, os.WriteFile(filepath.Join(made, "files.tar.gz"), raw, 0o600))
	require.ErrorContains(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: root}, filepath.Base(made)), "checksum")
	require.FileExists(t, marker)
	// Installable files paired with a broken database dump must roll back too.
	require.NoError(t, os.Rename(valid, filepath.Join(made, "files")))
	require.NoError(t, os.WriteFile(filepath.Join(made, "database.dump"), []byte("invalid dump"), 0o600))
	require.Error(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: root}, filepath.Base(made)))
	require.FileExists(t, marker)
	require.NoFileExists(t, filepath.Join(dest, "old.pdf"))
}
