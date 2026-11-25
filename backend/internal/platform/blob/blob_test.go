package blob

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir(), []byte("files-secret-0123456789abcdefghijk"), "http://api.test")
	require.NoError(t, err)
	return s
}

func get(s *Store, rawURL string) *httptest.ResponseRecorder {
	u, _ := url.Parse(rawURL)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u.RequestURI(), nil))
	return rec
}

func TestSignedLinksDownloadTheFile(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Put("tnt_a/doc_b", []byte("cv bytes")))
	rec := get(s, s.SignedURL("tnt_a/doc_b", time.Minute))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "cv bytes", rec.Body.String())
}

func TestLinksStopWorking(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Put("tnt_a/doc_b", []byte("cv")))
	link := s.SignedURL("tnt_a/doc_b", time.Minute)

	assert.Equal(t, http.StatusForbidden, get(s, strings.Replace(link, "sig=", "sig=f", 1)).Code, "tampered")
	assert.Equal(t, http.StatusForbidden, get(s, strings.Replace(link, "doc_b", "doc_c", 1)).Code, "other file")

	s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	assert.Equal(t, http.StatusForbidden, get(s, link).Code, "expired")
}

func TestKeysCannotEscapeTheStorageDirectory(t *testing.T) {
	s := newStore(t)
	for _, key := range []string{"../etc/passwd", "/abs", "a/../../b", "A/B", ""} {
		assert.ErrorIs(t, s.Put(key, []byte("x")), ErrInvalidKey, key)
	}
}

func TestFilesAreKeptInSubfolders(t *testing.T) {
	s := newStore(t)
	for i := range 300 {
		require.NoError(t, s.Put(fmt.Sprintf("tnt_a/doc_%d", i), []byte("cv")))
	}
	top, err := os.ReadDir(filepath.Join(s.dir, "tnt_a"))
	require.NoError(t, err)
	assert.Greater(t, len(top), 100, "spread over many subfolders")
	for _, e := range top {
		assert.True(t, e.IsDir(), "only subfolders in the agency's folder: %s", e.Name())
	}
	f, err := s.Open("tnt_a/doc_7")
	require.NoError(t, err)
	_ = f.Close()
}

func TestFilesSavedBeforeSubfoldersStillWork(t *testing.T) {
	s := newStore(t)
	require.NoError(t, os.MkdirAll(filepath.Join(s.dir, "tnt_a"), 0o750))
	old := filepath.Join(s.dir, "tnt_a", "doc_old")
	require.NoError(t, os.WriteFile(old, []byte("old cv"), 0o600))

	f, err := s.Open("tnt_a/doc_old")
	require.NoError(t, err)
	data, _ := io.ReadAll(f)
	_ = f.Close()
	assert.Equal(t, "old cv", string(data))
	assert.Equal(t, "old cv", get(s, s.SignedURL("tnt_a/doc_old", time.Minute)).Body.String())

	// Saving it again moves it into its subfolder; deleting removes it.
	require.NoError(t, s.Put("tnt_a/doc_old", []byte("new cv")))
	assert.NoFileExists(t, old)
	assert.Equal(t, "new cv", get(s, s.SignedURL("tnt_a/doc_old", time.Minute)).Body.String())
	require.NoError(t, os.WriteFile(old, []byte("stray"), 0o600))
	require.NoError(t, s.Delete("tnt_a/doc_old"))
	assert.NoFileExists(t, old)
	_, err = s.Open("tnt_a/doc_old")
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Equal(t, http.StatusNotFound, get(s, s.SignedURL("tnt_a/doc_old", time.Minute)).Code)
}

// A file that is there but cannot be opened (the restore left it to another
// user) is not "not found": the worker would give up on a file that a
// repair brings back. It is a 500, logged with the key's hash and the error
// number, never the file's path or the link's signature.
func TestAnUnreadableFileIsAServerErrorNotAMissingOne(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Put("tnt_a/doc_b", []byte("cv")))
	s.open = func(p string) (*os.File, error) { return nil, &fs.PathError{Op: "open", Path: p, Err: syscall.EACCES} }
	var logged bytes.Buffer
	link := s.SignedURL("tnt_a/doc_b", time.Minute)
	u, _ := url.Parse(link)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, u.RequestURI(), nil)
	req = req.WithContext(logx.Into(req.Context(), slog.New(slog.NewJSONHandler(&logged, nil))))
	s.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, logged.String(), `"errno":13`)
	assert.Contains(t, logged.String(), `"key_hash":"`)
	assert.NotContains(t, logged.String(), "doc_b", "never the key or path")
	assert.NotContains(t, logged.String(), u.Query().Get("sig"))

	_, err := s.Open("tnt_a/doc_b")
	require.ErrorIs(t, err, os.ErrPermission)
	assert.NotContains(t, err.Error(), s.dir, "the error names no path")
	assert.NotContains(t, err.Error(), "tnt_a")
}
