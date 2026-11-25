package backup_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// runtime is the user the API and workers run as in Docker.
var runtime = backup.Owner{UID: 65532, GID: 65532}

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("only root may give files to another user (the backup service in Docker runs as root)")
	}
}

// filesDir is a new files folder that other users can reach (as /data/files
// in Docker), unlike a test's own temporary folder.
func filesDir(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	for _, p := range []string{filepath.Dir(base), base} {
		require.NoError(t, os.Chmod(p, 0o711)) //nolint:gosec // entered, never listed
	}
	dir := filepath.Join(base, "files")
	require.NoError(t, os.Mkdir(dir, 0o755))
	return dir
}

func ownerOf(t *testing.T, p string) (uint32, uint32) {
	t.Helper()
	info, err := os.Lstat(p)
	require.NoError(t, err)
	st := info.Sys().(*syscall.Stat_t)
	return st.Uid, st.Gid
}

// madeBackup backs up a files folder holding one agency's CV (as root, like
// the backup service) and returns the backup's folder.
func madeBackup(t *testing.T) (made, cv string) {
	t.Helper()
	tenant := testutil.Current.NewTenant(t)
	files := t.TempDir()
	cv = filepath.Join(tenant.ID, "3f", "a9", "doc_1")
	require.NoError(t, os.MkdirAll(filepath.Join(files, filepath.Dir(cv)), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(files, cv), []byte("%PDF-1.7 synthetic cv"), 0o600))
	made, err := backup.Run(context.Background(), testutil.Current.Admin, backup.Options{
		AdminURL: testutil.Current.AdminURL, FilesDir: files, Dir: t.TempDir(), Keep: 2,
	})
	require.NoError(t, err)
	return made, cv
}

// BUG-002: a restore run as root left every folder root's and private, so
// the API (65532) could open no CV ("permission denied"). The restored
// files now go to the runtime user, stay private to it, open as it, and
// the backup's own copies are left exactly as they were.
func TestARestoreGivesTheFilesToTheUserTheAPIRunsAs(t *testing.T) {
	requireTools(t)
	requireRoot(t)
	ctx := context.Background()
	made, cv := madeBackup(t)
	dest := filesDir(t) // a new Docker volume: root's
	o := backup.Options{AdminURL: freshDatabase(t), FilesDir: dest, Dir: filepath.Dir(made), Owner: &runtime}
	require.NoError(t, backup.Restore(ctx, o, filepath.Base(made)))

	err := filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		uid, gid := ownerOf(t, p)
		assert.Equal(t, [2]uint32{65532, 65532}, [2]uint32{uid, gid}, p)
		info, err := d.Info()
		require.NoError(t, err)
		if p != dest { // the mount point keeps its mode
			assert.Zero(t, info.Mode().Perm()&0o077, "still private to its owner: %s %v", p, info.Mode())
		}
		return nil
	})
	require.NoError(t, err)

	check, err := backup.CheckFiles(ctx, dest, runtime, 5)
	require.NoError(t, err)
	assert.True(t, check.OK(), check.Summary())
	assert.Equal(t, 1, check.Sampled, "opened as 65532, through every folder on the way")

	restored, err := os.Stat(filepath.Join(dest, cv))
	require.NoError(t, err)
	kept, err := os.Stat(filepath.Join(made, "files", cv))
	require.NoError(t, err)
	assert.False(t, os.SameFile(restored, kept), "a copy of its own: changing it never changes the kept backup")
	uid, _ := ownerOf(t, filepath.Join(made, "files", cv))
	assert.EqualValues(t, 0, uid, "the kept backup's copy is untouched")
}

// A backup made before the files were a folder holds them as files.tar.gz:
// those are given to the runtime user too.
func TestALegacyTarRestoreGivesTheFilesToTheRuntimeUser(t *testing.T) {
	requireTools(t)
	requireRoot(t)
	ctx := context.Background()
	made, cv := madeBackup(t)
	packTar(t, filepath.Join(made, "files"), filepath.Join(made, "files.tar.gz"))
	require.NoError(t, os.RemoveAll(filepath.Join(made, "files")))

	dest := filesDir(t)
	o := backup.Options{AdminURL: freshDatabase(t), FilesDir: dest, Dir: filepath.Dir(made), Owner: &runtime}
	require.NoError(t, backup.Restore(ctx, o, filepath.Base(made)))
	for _, p := range []string{dest, filepath.Join(dest, filepath.Dir(cv)), filepath.Join(dest, cv)} {
		uid, gid := ownerOf(t, p)
		assert.Equal(t, [2]uint32{65532, 65532}, [2]uint32{uid, gid}, p)
	}
	check, err := backup.CheckFiles(ctx, dest, runtime, 5)
	require.NoError(t, err)
	assert.True(t, check.OK(), check.Summary())
}

// When the files cannot be given away, the restore stops before anything
// is replaced: the current files and database stay.
func TestARestoreThatCannotGiveTheFilesAwayChangesNothing(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	made, cv := madeBackup(t)
	dest := t.TempDir()
	current := filepath.Join(dest, "current.pdf")
	require.NoError(t, os.WriteFile(current, []byte("synthetic current file"), 0o600))
	backup.FailChown(t, syscall.EPERM)
	other := backup.Owner{UID: os.Geteuid() + 1, GID: os.Getegid() + 1}
	target := freshDatabase(t)
	err := backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: filepath.Dir(made), Owner: &other}, filepath.Base(made))
	require.ErrorIs(t, err, syscall.EPERM)
	assert.Contains(t, err.Error(), "HP_FILES_OWNER")
	assert.FileExists(t, current)
	assert.NoFileExists(t, filepath.Join(dest, cv))
	assert.NoDirExists(t, filepath.Join(dest, ".hp-restore"), "staging cleaned up")
	assert.Zero(t, tables(t, target, "tenant"), "the database was not replaced")
}

// Running as the runtime user already, the files are its own: no change asked.
func TestARestoreAsTheRuntimeUserChangesNoOwner(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	made, _ := madeBackup(t)
	backup.FailChown(t, errors.New("must not be called"))
	me := backup.Owner{UID: os.Geteuid(), GID: os.Getegid()}
	require.NoError(t, backup.Restore(ctx, backup.Options{AdminURL: freshDatabase(t), FilesDir: t.TempDir(), Dir: filepath.Dir(made), Owner: &me}, filepath.Base(made)))
}

// hp repair-files, for servers restored before: the check finds the files
// the runtime user cannot open; the repair gives them to it, makes nothing
// more open, and leaves a file with another link (a kept backup's) alone.
func TestRepairGivesUnreadableFilesBackWithoutTouchingLinkedCopies(t *testing.T) {
	requireRoot(t)
	ctx := context.Background()
	dir := filesDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tnt_a", "3f", "a9"), 0o700))
	cv := filepath.Join(dir, "tnt_a", "3f", "a9", "doc_1")
	require.NoError(t, os.WriteFile(cv, []byte("synthetic"), 0o600))
	open := filepath.Join(dir, "tnt_a", "3f", "a9", "doc_2")
	require.NoError(t, os.WriteFile(open, []byte("synthetic"), 0o644))
	require.NoError(t, os.Chmod(open, 0o644)) // past the umask
	kept := filepath.Join(t.TempDir(), "kept")
	linked := filepath.Join(dir, "tnt_a", "3f", "a9", "doc_3")
	require.NoError(t, os.WriteFile(kept, []byte("synthetic"), 0o600))
	require.NoError(t, os.Link(kept, linked))

	before, err := backup.CheckFiles(ctx, dir, runtime, 10)
	require.NoError(t, err)
	assert.False(t, before.OK())
	assert.Equal(t, 7, before.WrongOwner, "4 folders and 3 files")
	assert.Equal(t, 1, before.TooOpen)
	assert.Equal(t, 1, before.Linked)
	assert.Len(t, before.Unreadable, 3, "the folders are root's alone: 65532 opens none")

	skipped, err := backup.RepairFiles(dir, runtime)
	require.NoError(t, err)
	assert.Equal(t, 1, skipped)
	for _, p := range []string{dir, cv, open} {
		uid, _ := ownerOf(t, p)
		assert.EqualValues(t, 65532, uid, p)
	}
	info, err := os.Stat(open)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "other users lose access; nothing gets more open")
	uid, _ := ownerOf(t, kept)
	assert.EqualValues(t, 0, uid, "the linked copy is left alone")

	after, err := backup.CheckFiles(ctx, dir, runtime, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, after.WrongOwner, "only the linked copy")
	assert.Equal(t, []string{filepath.Join("tnt_a", "3f", "a9", "doc_3")}, after.Unreadable)
}

func TestOwnersAreReadAsUserAndGroupIDs(t *testing.T) {
	o, err := backup.ParseOwner("65532:65533")
	require.NoError(t, err)
	assert.Equal(t, backup.Owner{UID: 65532, GID: 65533}, o)
	assert.Equal(t, "65532:65533", o.String())
	for _, bad := range []string{"", "65532", "nonroot:nonroot", "-1:5", "1:2:3", "65532:"} {
		_, err := backup.ParseOwner(bad)
		assert.Error(t, err, bad)
	}
}

// packTar writes dir's files as the gzipped tar older backups held.
func packTar(t *testing.T, dir, out string) {
	t.Helper()
	f, err := os.Create(out) //nolint:gosec // a test's temporary folder
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	require.NoError(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // as above
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	}))
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
}

// tables reports whether the database at url has the table (1) or not (0).
func tables(t *testing.T, url, table string) int {
	t.Helper()
	pool, err := database.Connect(context.Background(), url)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_class WHERE relname = $1 AND relkind = 'r'`, table).Scan(&n))
	return n
}
