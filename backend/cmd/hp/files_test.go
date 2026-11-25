package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// stdout runs fn and returns what it printed.
func stdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stdout
	os.Stdout = w
	got := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		got <- string(b)
	}()
	defer func() { os.Stdout = saved }()
	fn()
	require.NoError(t, w.Close())
	return <-got
}

// yourself is this process's user, as HP_FILES_OWNER (repairing needs no root then).
func yourself() string { return strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()) }

// Review of 1.47.0: repairing the files gave them back, but the PDF copies
// that had failed meanwhile stayed "No PDF copy", and nothing said how to
// make them again.
func TestRepairFilesSaysHowToRemakeFailedPreviews(t *testing.T) {
	files := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(files, "tnt_1"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(files, "tnt_1", "doc_1"), []byte("cv"), 0o600))
	t.Setenv("HP_FILES_DIR", files)
	t.Setenv("HP_FILES_OWNER", yourself())
	t.Setenv("HP_DATABASE_ADMIN_URL", "")
	out := stdout(t, func() { require.NoError(t, run(context.Background(), "repair-files", []string{"-apply"})) })
	assert.Contains(t, out, "Make the PDF again")
}

// Review of 1.47.0: hp repair-files -apply ran (as root) while the API and
// workers did, writing files as it went. It refuses while they are
// connected to the database (unless -force), and says so.
func TestRepairFilesRefusesWhileTheAPIRuns(t *testing.T) {
	ctx := context.Background()
	files := t.TempDir()
	t.Setenv("HP_FILES_DIR", files)
	t.Setenv("HP_FILES_OWNER", yourself())
	t.Setenv("HP_DATABASE_ADMIN_URL", testutil.Current.AdminURL)
	api, err := testutil.Current.App.Acquire(ctx) // the API, connected
	require.NoError(t, err)
	defer func() {
		if api != nil {
			api.Release()
		}
	}()
	err = run(ctx, "repair-files", []string{"-apply"})
	require.ErrorContains(t, err, "stop them first")
	require.NoError(t, run(ctx, "repair-files", []string{"-apply", "-force"}))
	api.Release()
	api = nil
	testutil.Current.App.Reset() // its connections closed: the API stopped
	require.NoError(t, run(ctx, "repair-files", []string{"-apply"}))
}

// Review of 1.47.0: a restore whose files the runtime user cannot use
// (the check after it) said to run hp repair-files, not how in Docker.
func TestAFailedFilesCheckSaysExactlyWhatToRun(t *testing.T) {
	files := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(files, "doc_1"), []byte("cv"), 0o600))
	err := verifyRestoredFiles(context.Background(), backup.Options{FilesDir: files, Owner: &backup.Owner{UID: 4242, GID: 4242}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "with the API and workers still stopped")
	assert.Contains(t, err.Error(), "run --rm backup /app/hp repair-files -apply")
}
