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
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
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
	open    func(string) (*os.File, error) // os.Open; tests fake a failure
}

// New creates the directory if needed.
func New(dir string, secret []byte, baseURL string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("blob: %w", err)
	}
	return &Store{dir: dir, secret: secret, baseURL: strings.TrimRight(baseURL, "/"), now: time.Now, open: os.Open}, nil
}

// Put writes data under key atomically (a reader never sees half a file).
// A copy kept the old way (before subfolders) is replaced too.
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
	if err := os.Remove(s.flatPath(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("blob: %w", err)
	}
	return nil
}

// Delete removes key (wherever it is kept); a missing file is not an error.
func (s *Store) Delete(key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	for _, p := range []string{path, s.flatPath(key)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("blob: %w", err)
		}
	}
	return nil
}

// SignedURL returns a link that downloads key until ttl has passed.
func (s *Store) SignedURL(key string, ttl time.Duration) string {
	exp := strconv.FormatInt(s.now().Add(ttl).Unix(), 10)
	q := url.Values{"exp": {exp}, "sig": {s.sign(key, exp)}}
	return s.baseURL + PathPrefix + key + "?" + q.Encode()
}

// Renew returns a new link (lasting ttl) for the file of an old one that no
// longer works here: expired, or signed by another server (a backup restored
// from it). It returns false for a link that still works here (whatever
// refused it, a new one would be refused too) or that is not one of these
// file links.
func (s *Store) Renew(link string, ttl time.Duration) (string, bool) {
	u, err := url.Parse(link)
	if err != nil {
		return "", false
	}
	at := strings.Index(u.Path, PathPrefix)
	if at < 0 {
		return "", false
	}
	key := u.Path[at+len(PathPrefix):]
	if !keyRe.MatchString(key) {
		return "", false
	}
	exp, sig := u.Query().Get("exp"), u.Query().Get("sig")
	until, err := strconv.ParseInt(exp, 10, 64)
	works := err == nil && hmac.Equal([]byte(sig), []byte(s.sign(key, exp))) && s.now().Unix() < until
	if works {
		return "", false
	}
	return s.SignedURL(key, ttl), true
}

// Open returns the stored file for reading; the caller closes it. A file
// saved before subfolders is found where it was.
func (s *Store) Open(key string) (*os.File, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := s.open(path) // path is validated against keyRe and joined under s.dir
	if errors.Is(err, os.ErrNotExist) {
		if info, oldErr := os.Stat(s.flatPath(key)); oldErr == nil && info.Mode().IsRegular() {
			f, err = s.open(s.flatPath(key))
		}
	}
	if err != nil {
		return nil, openError(key, err)
	}
	return f, nil
}

// OpenError is why a stored file could not be opened, without its path (it
// names the agency and the document): the key's hash and the system's error
// number. errors.Is still finds os.ErrNotExist or os.ErrPermission in it.
type OpenError struct {
	KeyHash string        // the first 12 hex digits of the key's SHA-256
	Errno   syscall.Errno // 0 when the system gave none
	err     error
}

func openError(key string, err error) *OpenError {
	sum := sha256.Sum256([]byte(key))
	e := &OpenError{KeyHash: hex.EncodeToString(sum[:])[:12], err: err}
	_ = errors.As(err, &e.Errno)
	return e
}

func (e *OpenError) Error() string {
	why := "unknown error"
	if e.Errno != 0 {
		why = e.Errno.Error()
	}
	return fmt.Sprintf("blob: stored file %s could not be opened: %s", e.KeyHash, why)
}

func (e *OpenError) Unwrap() error { return e.err }

// LogValue keeps the key hash and the error number together in logs.
func (e *OpenError) LogValue() slog.Value {
	return slog.GroupValue(slog.String("key_hash", e.KeyHash), slog.Int("errno", int(e.Errno)), slog.String("error", e.Error()))
}

// MissingHeader marks ServeHTTP's own 404, the file not on this server
// ("missing"): the AI worker tells it from a 404 of something else.
const MissingHeader = "X-HyPlug-File"

// ServeHTTP serves GET /internal/files/{key}?exp=...&sig=... for workers.
func (s *Store) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, PathPrefix)
	exp, sig := r.URL.Query().Get("exp"), r.URL.Query().Get("sig")
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || s.now().Unix() > expUnix || !hmac.Equal([]byte(sig), []byte(s.sign(key, exp))) {
		http.Error(w, "invalid or expired link", http.StatusForbidden)
		return
	}
	f, err := s.Open(key)
	switch {
	case errors.Is(err, ErrInvalidKey):
		http.Error(w, "invalid or expired link", http.StatusForbidden)
		return
	case errors.Is(err, os.ErrNotExist): // gone for good: the worker gives up
		// Says it is this server's answer about the file (MissingHeader): a
		// 404 from anything else on the way (a wrong HP_INTERNAL_BASE_URL)
		// is a setting to fix, tried again, never "upload it again".
		w.Header().Set(MissingHeader, "missing")
		http.Error(w, "not found", http.StatusNotFound)
		return
	case err != nil: // there but unreadable (its owner, a disk error): fixable, so the worker tries again
		logx.From(r.Context()).Error("a stored file could not be opened for a worker", "file", err)
		http.Error(w, "the file could not be opened", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		logx.From(r.Context()).Error("a stored file could not be read for a worker", "file", openError(key, err))
		http.Error(w, "the file could not be opened", http.StatusInternalServerError)
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

// path is where key is kept: in the agency's folder, then two levels of
// subfolders picked from the key's hash ("tnt_x/3f/a9/doc_y"), so no folder
// holds more than a few dozen files even with millions of CVs (a folder with
// a million files is slow to open, back up and copy).
func (s *Store) path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:2])
	owner, rest, nested := strings.Cut(key, "/")
	if !nested {
		owner, rest = "", key
	}
	return filepath.Join(s.dir, owner, h[:2], h[2:], filepath.FromSlash(rest)), nil
}

// flatPath is where key was kept before subfolders (key already checked).
func (s *Store) flatPath(key string) string {
	return filepath.Join(s.dir, filepath.FromSlash(key))
}
