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
	"path/filepath"
	"strings"
)

// Restore stages every file on the destination volume before replacing the
// database in one transaction. Stop the API and workers before restoring.
func Restore(ctx context.Context, o Options, name string) (result error) {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%q is not a backup name (hiringplug-YYYYMMDD-HHMMSS)", name)
	}
	dir := filepath.Join(o.Dir, name)
	// The uploaded files are a folder; a backup made before that holds them
	// as files.tar.gz, and still restores.
	files, packed := filepath.Join(dir, filesFolder), filepath.Join(dir, "files.tar.gz")
	inFolder := false
	if info, err := os.Stat(files); err == nil && info.IsDir() {
		inFolder = true
	}
	need := []string{"database.dump", "manifest.json"}
	if !inFolder {
		need = append(need, "files.tar.gz")
	}
	for _, f := range need {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			return fmt.Errorf("backup %s is incomplete: %w", name, err)
		}
	}
	major, err := serverMajor(ctx, o.AdminURL)
	if err != nil {
		return err
	}
	for _, tool := range []string{"pg_restore", "psql"} { // both are needed: find out now, before anything moves
		if _, err := o.tools().tool(tool, major); err != nil {
			return err
		}
	}
	if o.FilesDir == "" {
		return errors.New("a files folder is required")
	}
	if err := os.MkdirAll(o.FilesDir, 0o700); err != nil {
		return err
	}
	// Keep the volume root: it may be a Docker mount point.
	source, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	dest, err := filepath.EvalSymlinks(o.FilesDir)
	if err != nil {
		return err
	}
	for _, pair := range [][2]string{{source, dest}, {dest, source}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return errors.New("backup and destination folders must be separate")
		}
	}
	stage := filepath.Join(o.FilesDir, stageName)
	if err := os.Mkdir(stage, 0o700); err != nil {
		return fmt.Errorf("cannot stage restore at %s (check for an interrupted restore): %w", stage, err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			if err := os.RemoveAll(stage); err != nil {
				result = errors.Join(result, fmt.Errorf("cleaning restore staging directory: %w", err))
			}
		}
	}()
	next, old := filepath.Join(stage, "next"), filepath.Join(stage, "previous")
	for _, d := range []string{next, old} {
		if err := os.Mkdir(d, 0o700); err != nil {
			return err
		}
	}
	if inFolder {
		err = copyTree(files, next)
	} else {
		err = untar(packed, next)
	}
	if err != nil {
		return fmt.Errorf("staging backup files: %w", err)
	}
	if err := checkFileCount(filepath.Join(dir, "manifest.json"), next); err != nil {
		return err
	}
	// The files go to the user the API and workers run as before they are
	// put in place: should that fail, nothing has changed yet.
	if err := o.giveRestored(next); err != nil {
		return err
	}
	previous, err := os.ReadDir(o.FilesDir)
	if err != nil {
		return err
	}
	incoming, err := os.ReadDir(next)
	if err != nil {
		return err
	}
	var moves [][2]string
	move := func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		moves = append(moves, [2]string{from, to})
		return nil
	}
	undo := func(cause error) error {
		for i := len(moves) - 1; i >= 0; i-- {
			if err := os.Rename(moves[i][1], moves[i][0]); err != nil {
				cleanup = false
				return errors.Join(cause, fmt.Errorf("recover previous files from %s: %w", stage, err))
			}
		}
		return cause
	}
	for _, e := range previous {
		if e.Name() != stageName {
			if err := move(filepath.Join(o.FilesDir, e.Name()), filepath.Join(old, e.Name())); err != nil {
				return undo(err)
			}
		}
	}
	for _, e := range incoming {
		if e.Name() == stageName {
			return undo(errors.New("reserved restore directory in backup"))
		}
		if err := move(filepath.Join(next, e.Name()), filepath.Join(o.FilesDir, e.Name())); err != nil {
			return undo(err)
		}
	}
	if err := restoreDatabase(ctx, o, major, filepath.Join(dir, "database.dump")); err != nil {
		return undo(err)
	}
	return nil
}

// giveRestored gives the staged files, and the files folder itself (a new
// Docker volume starts as root's), to Owner: the restore runs as root in
// Docker, the API as another user. Nothing to do without an Owner, or when
// this process already runs as that user (its files are its own).
func (o Options) giveRestored(staged string) error {
	if o.Owner == nil || os.Geteuid() == o.Owner.UID {
		return nil
	}
	_, err := giveFiles(staged, *o.Owner, false) // copies of their own: one link each
	if err == nil {
		err = lchown(o.FilesDir, o.Owner.UID, o.Owner.GID)
	}
	if err != nil {
		return fmt.Errorf("giving the restored files to %s (HP_FILES_OWNER; restore as root, as the backup service does, or leave it unset when the API runs as you): %w", o.Owner, err)
	}
	return nil
}

func checkFileCount(manifest, staged string) error {
	raw, err := os.ReadFile(manifest) //nolint:gosec // own named backup manifest
	if err != nil {
		return err
	}
	var info struct {
		Files *int `json:"files"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return err
	}
	if info.Files == nil {
		return nil
	} // legacy manifest without a count
	count := 0
	err = filepath.WalkDir(staged, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			count++
		}
		return err
	})
	if err == nil && count != *info.Files {
		return fmt.Errorf("backup lists %d files but contains %d", *info.Files, count)
	}
	return err
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
			// Read through the gzip trailer to verify its checksum.
			const maxPadding = 1 << 20
			n, err := io.Copy(io.Discard, io.LimitReader(gz, maxPadding+1))
			if n > maxPadding {
				return errors.New("excess padding after the files archive")
			}
			return err
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
