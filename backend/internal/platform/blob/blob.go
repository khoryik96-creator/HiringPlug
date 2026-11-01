// Package blob stores uploaded files (CVs) and hands out short-lived signed
// download links.
//
// Workers never get storage credentials: a job carries a signed URL, which
// only works for one file until it expires. This store keeps files on the
// local disk (local-first deployment). When moving to the cloud, an S3
// implementation can return S3 presigned URLs from SignedURL instead, and the
// workers do not change.
package blob

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PathPrefix is where the API serves signed downloads.
const PathPrefix = "/internal/files/"

var keyRe = regexp.MustCompile(`^[a-z0-9_]+(/[a-z0-9_]+)*$`)

// ErrInvalidKey is returned for keys that could escape the storage directory.
var ErrInvalidKey = errors.New("blob: invalid key")

// Store keeps files under a directory.
type Store struct {
	dir     string
	secret  []byte
	baseURL string
	now     func() time.Time
}

// New creates the directory if needed.
func New(dir string, secret []byte, baseURL string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("blob: %w", err)
	}
	return &Store{dir: dir, secret: secret, baseURL: strings.TrimRight(baseURL, "/"), now: time.Now}, nil
}

// Put writes data under key atomically (a reader never sees half a file).
func (s *Store) Put(key string, data []byte) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("blob: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return fmt.Errorf("blob: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("blob: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("blob: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("blob: %w", err)
	}
	return nil
}

// Delete removes key; a missing file is not an error.
func (s *Store) Delete(key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("blob: %w", err)
	}
	return nil
}

// SignedURL returns a link that downloads key until ttl has passed.
func (s *Store) SignedURL(key string, ttl time.Duration) string {
	exp := strconv.FormatInt(s.now().Add(ttl).Unix(), 10)
	q := url.Values{"exp": {exp}, "sig": {s.sign(key, exp)}}
	return s.baseURL + PathPrefix + key + "?" + q.Encode()
}

// ServeHTTP serves GET /internal/files/{key}?exp=...&sig=... for workers.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, PathPrefix)
	exp, sig := r.URL.Query().Get("exp"), r.URL.Query().Get("sig")
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || s.now().Unix() > expUnix || !hmac.Equal([]byte(sig), []byte(s.sign(key, exp))) {
		http.Error(w, "invalid or expired link", http.StatusForbidden)
		return
	}
	path, err := s.path(key)
	if err != nil {
		http.Error(w, "invalid or expired link", http.StatusForbidden)
		return
	}
	f, err := os.Open(path) //nolint:gosec // path is validated against keyRe and joined under s.dir
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (s *Store) sign(key, exp string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(key + "\n" + exp))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Store) path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(s.dir, filepath.FromSlash(key)), nil
}
