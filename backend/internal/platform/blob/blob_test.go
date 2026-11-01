package blob

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
