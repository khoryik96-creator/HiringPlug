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

// requireRoot runs a test only as root (only root may give files to
// another user; the backup service in Docker runs as root). Never skipped
// silently in CI: CI's main test step (HP_TEST_ROOT=later) leaves them to
// the step that runs them as root, and anywhere else in CI (CI or HP_CI
// set) not being root fails.
func requireRoot(t *testing.T) {
	t.Helper()
	switch {
	case os.Geteuid() == 0:
	case os.Getenv("HP_TEST_ROOT") == "later":
		t.Skip("not root: CI runs this again as root, in the step after (.github/workflows/ci.yml)")
	case os.Getenv("CI") != "" || os.Getenv("HP_CI") != "":
		t.Fatal("not root: in CI this must run as root (sudo), as the backup service does; see .github/workflows/ci.yml")
	default:
		t.Skip("only root may give files to another user (the backup service in Docker runs as root): run as root, as CI does")
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

// Review of 1.47.0: hp repair-files runs as root while the API may still
// run (as 65532, owning the files folder): a folder or file it swapped for
// a link between the walk listing it and changing it was followed, and
// root changed what the link pointed to, outside the files folder. Links
// are never followed now, whenever they appear.
func TestRepairNeverFollowsALinkSwappedInWhileItRuns(t *testing.T) {
	requireRoot(t)
	dir := filesDir(t)
	outside := t.TempDir()
	secret, sub := filepath.Join(outside, "secret"), filepath.Join(outside, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	inner := filepath.Join(sub, "inner")
	for _, p := range []string{secret, inner} {
		require.NoError(t, os.WriteFile(p, []byte("not HiringPlug's"), 0o644))
		require.NoError(t, os.Chmod(p, 0o644))
	}
	for _, p := range []string{secret, sub, inner} {
		require.NoError(t, os.Lchown(p, 1234, 1234))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tnt_a", "3f"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tnt_a", "3f", "doc_1"), []byte("cv"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tnt_a", "doc_2"), []byte("cv"), 0o644))
	require.NoError(t, os.Chmod(filepath.Join(dir, "tnt_a", "doc_2"), 0o644))
	swap := func(p, to string) {
		require.NoError(t, os.Rename(p, p+".moved"))
		require.NoError(t, os.Symlink(to, p))
	}
	backup.BeforeOpen(t, func(d, name string) {
		switch name {
		case "3f": // a folder, listed, becomes a link to another folder
			swap(filepath.Join(d, name), sub)
		case "doc_2": // a file, listed, becomes a link to another file
			swap(filepath.Join(d, name), secret)
		}
	})

	_, err := backup.RepairFiles(dir, runtime)
	require.NoError(t, err)
	for _, p := range []string{secret, sub, inner} {
		uid, gid := ownerOf(t, p)
		assert.Equal(t, [2]uint32{1234, 1234}, [2]uint32{uid, gid}, "never followed: %s", p)
	}
	for _, p := range []string{secret, inner} {
		info, err := os.Stat(p)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), p)
	}
	uid, _ := ownerOf(t, filepath.Join(dir, "tnt_a"))
	assert.EqualValues(t, 65532, uid, "the rest is repaired")
}

// Review of 1.47.0: a file was opened as the runtime user with root's
// supplementary groups kept (NoSetGroups), so a folder only root's group
// may enter passed the check, while the API (65532 alone) cannot open it.
func TestFilesAreOpenedAsTheRuntimeUserAloneWithoutRootsGroups(t *testing.T) {
	requireRoot(t)
	groups, err := syscall.Getgroups()
	require.NoError(t, err)
	require.NoError(t, syscall.Setgroups([]int{0})) // root in its own group, as under sudo
	t.Cleanup(func() { _ = syscall.Setgroups(groups) })
	dir := filesDir(t)
	private := filepath.Join(dir, "tnt_g")
	require.NoError(t, os.Mkdir(private, 0o750))
	require.NoError(t, os.Chmod(private, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(private, "doc"), []byte("cv"), 0o640))
	check, err := backup.CheckFiles(context.Background(), dir, runtime, 10)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join("tnt_g", "doc")}, check.Unreadable, "65532 alone cannot get in")
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
