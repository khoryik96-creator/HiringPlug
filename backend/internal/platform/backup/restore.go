package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Restore puts backup name (a folder under o.Dir) back: the database first
// (everything in it is replaced), then the files (the files folder is
// emptied and refilled). Stop the API and workers before restoring.
func Restore(ctx context.Context, o Options, name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%q is not a backup name (hiringplug-YYYYMMDD-HHMMSS)", name)
	}
	dir := filepath.Join(o.Dir, name)
	for _, f := range []string{"database.dump", "files.tar.gz", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return fmt.Errorf("backup %s is incomplete: %w", name, err)
		}
	}
	out, err := exec.CommandContext(ctx, "pg_restore", "--clean", "--if-exists", "--single-transaction", //nolint:gosec // fixed program; our own setting and file
		"--dbname", o.AdminURL, filepath.Join(dir, "database.dump")).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_restore (the database is unchanged): %w: %s", err, hidePassword(string(out), o.AdminURL))
	}
	if err := emptyDir(o.FilesDir); err != nil {
		return fmt.Errorf("emptying the files folder: %w", err)
	}
	return untar(filepath.Join(dir, "files.tar.gz"), o.FilesDir)
}

func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		errs = append(errs, os.RemoveAll(filepath.Join(dir, e.Name())))
	}
	return errors.Join(errs...)
}

// untar extracts regular files only, and only inside dir.
func untar(path, dir string) error {
	f, err := os.Open(path) //nolint:gosec // a backup's own file
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return fmt.Errorf("refusing %q: outside the files folder", hdr.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // checked to be inside dir
		if err != nil {
			return err
		}
		_, err = io.Copy(out, io.LimitReader(tr, hdr.Size))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
}
