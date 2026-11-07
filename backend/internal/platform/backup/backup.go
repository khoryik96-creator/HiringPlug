// Package backup copies the whole server, the database and the uploaded
// files, into a folder of its own, keeps the newest few, and puts one back.
//
// A backup is a folder named hiringplug-YYYYMMDD-HHMMSS holding
// database.dump (pg_dump's custom format), files.tar.gz and manifest.json.
// It is written under a hidden name and renamed when complete, so a folder
// with the usual name is always a whole backup. Each run is recorded in
// backup_run (Settings > System shows the last one).
//
// It needs pg_dump and pg_restore of the server's major version; the backup
// service's image (deploy/backup.Dockerfile) has them.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options say what to back up and where.
type Options struct {
	AdminURL string // the database, as its owner (pg_dump needs to read every table)
	FilesDir string // the uploaded files (HP_FILES_DIR)
	Dir      string // where backups go (HP_BACKUP_DIR)
	Keep     int    // how many backups to keep (older ones are deleted)
	Now      func() time.Time
	finder   *toolFinder // tests
}

func (o Options) tools() toolFinder {
	if o.finder != nil {
		return *o.finder
	}
	return systemTools
}

var nameRe = regexp.MustCompile(`^hiringplug-\d{8}-\d{6}$`)

// Last is the latest backup run, for Settings > System.
type Last struct {
	Name       string
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string // running, done or failed
	SizeBytes  *int64
	Error      *string
}

// LastRun returns the latest backup run; ok is false when there has been none.
func LastRun(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (l Last, ok bool, err error) {
	err = db.QueryRow(ctx, `SELECT name, started_at, finished_at, status, size_bytes, error
		FROM backup_run ORDER BY started_at DESC, id DESC LIMIT 1`).
		Scan(&l.Name, &l.StartedAt, &l.FinishedAt, &l.Status, &l.SizeBytes, &l.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return Last{}, false, nil
	}
	return l, err == nil, err
}

// Run makes one backup, records it, and deletes those beyond Keep. It returns
// the new backup's folder.
func Run(ctx context.Context, admin *pgxpool.Pool, o Options) (string, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Keep < 1 {
		o.Keep = 1
	}
	name := "hiringplug-" + o.Now().UTC().Format("20060102-150405")
	var runID int64
	if err := admin.QueryRow(ctx, `INSERT INTO backup_run (name, started_at, status) VALUES ($1, now(), 'running') RETURNING id`,
		name).Scan(&runID); err != nil {
		return "", fmt.Errorf("recording the backup: %w", err)
	}
	dir, size, err := write(ctx, o, name)
	finish := `UPDATE backup_run SET finished_at = now(), status = 'done', size_bytes = $2 WHERE id = $1`
	args := []any{runID, size}
	if err != nil {
		finish = `UPDATE backup_run SET finished_at = now(), status = 'failed', error = $2 WHERE id = $1`
		args = []any{runID, clip(err.Error(), 2000)}
	}
	// Recorded even if ctx was cancelled meanwhile.
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, recErr := admin.Exec(recCtx, finish, args...); recErr != nil {
		err = errors.Join(err, fmt.Errorf("recording the backup: %w", recErr))
	}
	if err != nil {
		return "", err
	}
	return dir, prune(o.Dir, o.Keep)
}

func write(ctx context.Context, o Options, name string) (string, int64, error) {
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return "", 0, err
	}
	tmp := filepath.Join(o.Dir, "."+name+".partial")
	final := filepath.Join(o.Dir, name)
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return "", 0, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()
	major, err := serverMajor(ctx, o.AdminURL)
	if err != nil {
		return "", 0, err
	}
	pgDump, err := o.tools().tool("pg_dump", major)
	if err != nil {
		return "", 0, err
	}
	dump := filepath.Join(tmp, "database.dump")
	if out, err := exec.CommandContext(ctx, pgDump, "--format=custom", "--file", dump, "--dbname", o.AdminURL).CombinedOutput(); err != nil { //nolint:gosec // a PostgreSQL tool; the URL is our own setting
		return "", 0, fmt.Errorf("pg_dump: %w: %s", err, hidePassword(string(out), o.AdminURL))
	}
	if err := os.Chmod(dump, 0o600); err != nil { // it holds every company's data
		return "", 0, err
	}
	files, err := tarFiles(o.FilesDir, filepath.Join(tmp, "files.tar.gz"))
	if err != nil {
		return "", 0, fmt.Errorf("copying the files: %w", err)
	}
	size, err := dirSize(tmp)
	if err != nil {
		return "", 0, err
	}
	manifest, _ := json.MarshalIndent(map[string]any{
		"name": name, "createdAt": o.Now().UTC(), "files": files, "sizeBytes": size,
		"restore": "hp restore " + name + " --yes (stop the API and workers first)",
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), manifest, 0o600); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmp, final); err != nil {
		return "", 0, err
	}
	ok = true
	return final, size, nil
}

// tarFiles writes every regular file under dir into a gzipped tar at path. It
// returns how many there were.
func tarFiles(dir, path string) (int, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // a new file in the backup being written
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	n := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir // no files yet
			}
			return err
		}
		if !d.Type().IsRegular() { // folders are implied; links are never followed
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o600, Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		src, err := os.Open(p) //nolint:gosec // a file under our own files folder
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, src)
		_ = src.Close()
		n++
		return err
	})
	if err != nil {
		return 0, err
	}
	if err := tw.Close(); err != nil {
		return 0, err
	}
	if err := gz.Close(); err != nil {
		return 0, err
	}
	return n, f.Close()
}

// prune deletes the oldest backups beyond keep, and partial ones left by a crash.
func prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	var errs []error
	for _, e := range entries {
		switch {
		case e.IsDir() && nameRe.MatchString(e.Name()):
			names = append(names, e.Name())
		case e.IsDir() && strings.HasPrefix(e.Name(), ".hiringplug-") && strings.HasSuffix(e.Name(), ".partial"):
			errs = append(errs, os.RemoveAll(filepath.Join(dir, e.Name())))
		}
	}
	slices.Sort(names) // the names sort by time
	for len(names) > keep {
		errs = append(errs, os.RemoveAll(filepath.Join(dir, names[0])))
		names = names[1:]
	}
	return errors.Join(errs...)
}

func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return err
	})
	return total, err
}

// hidePassword keeps the database password out of error messages.
func hidePassword(s, url string) string {
	if i := strings.Index(url, "://"); i >= 0 {
		if at := strings.LastIndex(url, "@"); at > i {
			if colon := strings.Index(url[i+3:at], ":"); colon >= 0 {
				pw := url[i+3+colon+1 : at]
				if pw != "" {
					s = strings.ReplaceAll(s, pw, "***")
				}
			}
		}
	}
	return clip(s, 1000)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
