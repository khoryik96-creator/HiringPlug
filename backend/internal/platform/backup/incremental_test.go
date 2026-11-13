package backup_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
)

func write(t *testing.T, path, content string, mod time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chtimes(path, mod, mod))
}

// Each night only the files new (or changed) since the backup before are
// copied; the rest are shared with it, and every backup stays whole when
// another is deleted.
func TestABackupCopiesOnlyTheFilesNewSinceTheOneBefore(t *testing.T) {
	src, backups := t.TempDir(), t.TempDir()
	day1 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	write(t, filepath.Join(src, "tnt_a", "doc_1"), "first cv", day1)
	write(t, filepath.Join(src, "tnt_a", "doc_2"), "second cv", day1)

	first := filepath.Join(backups, "hiringplug-20261101-020000", "files")
	n, copied, _, err := backup.LinkFiles(src, backup.PreviousFiles(backups), first, "hiringplug-20261101-020000/files")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.EqualValues(t, len("first cv")+len("second cv"), copied, "the first backup copies everything")

	write(t, filepath.Join(src, "tnt_a", "doc_2"), "second cv, replaced", day1.Add(time.Hour))
	write(t, filepath.Join(src, "tnt_a", "doc_3"), "third cv", day1.Add(time.Hour))
	assert.Equal(t, first, backup.PreviousFiles(backups))
	second := filepath.Join(backups, "hiringplug-20261102-020000", "files")
	n, copied, _, err = backup.LinkFiles(src, backup.PreviousFiles(backups), second, "hiringplug-20261102-020000/files")
	require.NoError(t, err)
	assert.Equal(t, 3, n, "it holds every file")
	assert.EqualValues(t, len("second cv, replaced")+len("third cv"), copied, "but copied only the new and the changed")

	same := func(name string) bool {
		a, err := os.Stat(filepath.Join(first, "tnt_a", name))
		require.NoError(t, err)
		b, err := os.Stat(filepath.Join(second, "tnt_a", name))
		require.NoError(t, err)
		return os.SameFile(a, b)
	}
	assert.True(t, same("doc_1"), "unchanged: one copy on the disk for both backups")
	assert.False(t, same("doc_2"), "changed: each backup keeps its own")
	old, err := os.ReadFile(filepath.Join(first, "tnt_a", "doc_2"))
	require.NoError(t, err)
	assert.Equal(t, "second cv", string(old), "the older backup still has the file as it was")

	require.NoError(t, os.RemoveAll(filepath.Join(backups, "hiringplug-20261101-020000")))
	kept, err := os.ReadFile(filepath.Join(second, "tnt_a", "doc_1"))
	require.NoError(t, err)
	assert.Equal(t, "first cv", string(kept), "deleting the older backup leaves the newer one whole")
}

// The size shown is the download's size to the byte, with the uploaded files
// in their folder and names longer than a tar header holds.
func TestADownloadWithManyFilesIsExactlyTheSizeShown(t *testing.T) {
	dir := t.TempDir()
	name := "hiringplug-20261101-020000"
	path := filepath.Join(dir, name)
	mod := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	write(t, filepath.Join(path, "database.dump"), strings.Repeat("d", 1000), mod)
	src := t.TempDir()
	long := filepath.Join("tnt_01jbz6w0m0k8q9x2r3s4t5v6w7", "emails", "attachments", "att_01jbz6w0m0k8q9x2r3s4t5v6w7_"+strings.Repeat("x", 60))
	write(t, filepath.Join(src, "tnt_a", "doc_1"), strings.Repeat("a", 513), mod)
	write(t, filepath.Join(src, long), strings.Repeat("b", 2048), mod)
	write(t, filepath.Join(src, "tnt_a", "empty"), "", mod)
	n, copied, tarBytes, err := backup.LinkFiles(src, "", filepath.Join(path, "files"), name+"/files")
	require.NoError(t, err)
	require.Equal(t, 3, n)

	check := func(manifest string) {
		t.Helper()
		write(t, filepath.Join(path, "manifest.json"), manifest, mod)
		size, err := backup.Size(dir, name)
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, backup.WriteTar(&buf, dir, name))
		assert.EqualValues(t, buf.Len(), size, "manifest %s", manifest)
	}
	check(`{"files":3}`)                                                                            // counted from the folder
	check(`{"files":3,"newFileBytes":` + itoa(copied) + `,"filesTarBytes":` + itoa(tarBytes) + `}`) // as the backup wrote it down
}

func itoa(n int64) string {
	var b []byte
	if n == 0 {
		return "0"
	}
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
