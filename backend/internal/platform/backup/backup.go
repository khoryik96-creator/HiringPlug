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
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// rebuilt are the tables a backup keeps empty: what search by meaning read
// of each candidate and search (tens of KB per candidate, as numbers), which
// its read-through reads again after a restore, from the CVs and profiles.
var rebuilt = []string{"meaning_candidate", "meaning_role", "meaning_passage", "meaning_pending", "meaning_query", "meaning_query_need", "meaning_sweep"}

// Options say what to back up and where.
type Options struct {
	AdminURL string  // the database, as its owner (pg_dump needs to read every table)
	FilesDir string  // the uploaded files (HP_FILES_DIR)
	Dir      string  // where backups go (HP_BACKUP_DIR)
	Keep     int     // how many backups to keep (older ones are deleted)
	Offsite  Offsite // where each backup is also copied, off this server (zero: not set up)
	// Server names the database server (ServerID); "" asks the database.
	Server string
	// Owner is who restored files are given to (HP_FILES_OWNER): the user
	// the API and workers run as. nil: they stay this process's own.
	Owner  *Owner
	Now    func() time.Time
	finder *toolFinder // tests
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
	// Its copy off the server: done, failed, off (none set up) or nil (not tried yet).
	OffsiteStatus *string
	OffsiteAt     *time.Time
	OffsiteError  *string
}

// LastRun returns the latest backup run; ok is false when there has been none.
func LastRun(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (l Last, ok bool, err error) {
	err = db.QueryRow(ctx, `SELECT name, started_at, finished_at, status, size_bytes, error, offsite_status, offsite_at, offsite_error
		FROM backup_run ORDER BY started_at DESC, id DESC LIMIT 1`).
		Scan(&l.Name, &l.StartedAt, &l.FinishedAt, &l.Status, &l.SizeBytes, &l.Error, &l.OffsiteStatus, &l.OffsiteAt, &l.OffsiteError)
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
	var dir string
	var size int64
	var err error
	if o.Server == "" { // a failure here is recorded like any other
		o.Server, err = ServerID(ctx, o.AdminURL)
	}
	if err == nil {
		dir, size, err = write(ctx, o, name)
	}
	if err == nil { // read back now, not on the day it is needed
		if _, verr := Verify(ctx, o, name); verr != nil {
			err = fmt.Errorf("the backup was made but does not read back: %w", verr)
		}
	}
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
	if err := prune(o.Dir, o.Keep); err != nil {
		return dir, err
	}
	return dir, offsite(ctx, admin, o, runID, name)
}

// offsite copies the new backup off the server and records how it went.
func offsite(ctx context.Context, admin *pgxpool.Pool, o Options, runID int64, name string) error {
	status, msg := "off", ""
	var err error
	if o.Offsite.On() {
		status = "done"
		if err = copyOffsite(ctx, o.Offsite, o.Dir, name, o.Server, o.Keep); err != nil {
			status, msg = "failed", clip(err.Error(), 2000)
		}
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, recErr := admin.Exec(recCtx, `UPDATE backup_run SET offsite_status = $2, offsite_at = now(), offsite_error = NULLIF($3, '') WHERE id = $1`,
		runID, status, msg); recErr != nil {
		err = errors.Join(err, fmt.Errorf("recording the off-site copy: %w", recErr))
	}
	if err != nil {
		return fmt.Errorf("the backup %s is made, but its copy off the server failed: %w", name, err)
	}
	return nil
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
	args := []string{"--format=custom", "--file", dump}
	for _, table := range rebuilt {
		args = append(args, "--exclude-table-data="+table)
	}
	args = append(args, "--dbname", o.AdminURL)
	if out, err := exec.CommandContext(ctx, pgDump, args...).CombinedOutput(); err != nil { //nolint:gosec // a PostgreSQL tool; the URL is our own setting
		return "", 0, fmt.Errorf("pg_dump: %w: %s", err, hidePassword(string(out), o.AdminURL))
	}
	if err := os.Chmod(dump, 0o600); err != nil { // it holds every company's data
		return "", 0, err
	}
	// Only files new since the previous backup are copied; the rest are
	// shared with it (incremental.go).
	files, copied, filesTar, err := linkFiles(o.FilesDir, previousFiles(o.Dir), filepath.Join(tmp, filesFolder), name+"/"+filesFolder)
	if err != nil {
		return "", 0, fmt.Errorf("copying the files: %w", err)
	}
	size, err := dirSize(tmp)
	if err != nil {
		return "", 0, err
	}
	manifest, _ := json.MarshalIndent(map[string]any{
		"name": name, "server": o.Server, "createdAt": o.Now().UTC(), "files": files, "sizeBytes": size,
		"newFileBytes": copied, "filesTarBytes": filesTar,
		"restore": "hp restore -yes " + name + " (stop the API and workers first)",
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
