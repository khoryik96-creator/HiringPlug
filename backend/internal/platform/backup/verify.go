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
	"strings"
)

// Verified is what a backup was found to hold.
type Verified struct {
	Files  int // uploaded files
	Tables int // tables with data in the database copy
}

// Verify checks that a backup can be put back, without putting it back
// (the server keeps running): every part is there, the database copy reads
// to the end (pg_restore lists it), and the uploaded files are all there,
// as many as its manifest lists. A backup made, then damaged or cut short
// on its disk or on its way off the server, is found now, not on the day it
// is needed.
func Verify(ctx context.Context, o Options, name string) (Verified, error) {
	var out Verified
	if !nameRe.MatchString(name) {
		return out, fmt.Errorf("%q is not a backup name (hiringplug-YYYYMMDD-HHMMSS)", name)
	}
	dir := filepath.Join(o.Dir, name)
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json")) //nolint:gosec // a named backup's own file
	if err != nil {
		return out, fmt.Errorf("backup %s has no manifest: %w", name, err)
	}
	var manifest struct {
		Files *int `json:"files"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return out, fmt.Errorf("backup %s: its manifest cannot be read: %w", name, err)
	}
	if out.Files, err = countFiles(dir); err != nil {
		return out, fmt.Errorf("backup %s: its files cannot be read: %w", name, err)
	}
	if manifest.Files != nil && out.Files != *manifest.Files {
		return out, fmt.Errorf("backup %s lists %d files but holds %d", name, *manifest.Files, out.Files)
	}
	major, err := serverMajor(ctx, o.AdminURL)
	if err != nil {
		return out, err
	}
	pgRestore, err := o.tools().tool("pg_restore", major)
	if err != nil {
		return out, err
	}
	list, err := exec.CommandContext(ctx, pgRestore, "--list", filepath.Join(dir, "database.dump")).Output() //nolint:gosec // a PostgreSQL tool on a named backup's own file
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return out, fmt.Errorf("backup %s: its database copy cannot be read: %w", name, err)
	}
	for line := range strings.SplitSeq(string(list), "\n") {
		if strings.Contains(line, " TABLE DATA ") {
			out.Tables++
		}
	}
	if out.Tables == 0 {
		return out, fmt.Errorf("backup %s: its database copy holds no tables", name)
	}
	return out, nil
}

// countFiles counts a backup's uploaded files: its files folder, or the
// files.tar.gz of a backup made before folders, read to its end.
func countFiles(dir string) (int, error) {
	folder := filepath.Join(dir, filesFolder)
	if info, err := os.Stat(folder); err == nil && info.IsDir() {
		n := 0
		err := filepath.WalkDir(folder, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				n++
			}
			return err
		})
		return n, err
	}
	f, err := os.Open(filepath.Join(dir, "files.tar.gz")) //nolint:gosec // a backup's own file
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	r, n := tar.NewReader(gz), 0
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		if h.Typeflag == tar.TypeReg {
			n++
		}
		if _, err := io.Copy(io.Discard, r); err != nil { //nolint:gosec // read to check it, not kept
			return n, err
		}
	}
}
