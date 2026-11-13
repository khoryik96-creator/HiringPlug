package backup

// The backups on disk, as Settings shows them: each one's size and when it
// was made, one deleted, and one packed into a single .tar file to download.

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// ErrNotFound: no complete backup by that name.
var ErrNotFound = errors.New("backup: not found")

// Info is one complete backup on disk.
type Info struct {
	Name      string
	CreatedAt time.Time
	SizeBytes int64 // the .tar download's size
	Files     int   // uploaded files it holds
	// DiskBytes is what it added to the disk: files unchanged since the
	// backup before it are shared with that one and not counted again.
	DiskBytes int64
}

// ValidName reports whether name is a backup's folder name (no paths).
func ValidName(name string) bool { return nameRe.MatchString(name) }

// List returns the complete backups in dir, newest first. A missing dir
// holds none.
func List(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range entries {
		if !e.IsDir() || !nameRe.MatchString(e.Name()) {
			continue
		}
		info, err := Stat(dir, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	slices.SortFunc(out, func(a, b Info) int { return -compareNames(a.Name, b.Name) })
	return out, nil
}

func compareNames(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Stat describes one backup: when it was made (its manifest, or its name),
// how many files it holds and the size of its .tar download.
func Stat(dir, name string) (Info, error) {
	if !nameRe.MatchString(name) {
		return Info{}, ErrNotFound
	}
	path := filepath.Join(dir, name)
	parts, err := parts(path)
	if err != nil {
		return Info{}, err
	}
	out := Info{Name: name, SizeBytes: tarSize(name, parts)}
	for _, p := range parts {
		out.DiskBytes += p.size
	}
	out.CreatedAt, _ = time.Parse("20060102-150405", name[len("hiringplug-"):])
	var m struct {
		CreatedAt     time.Time `json:"createdAt"`
		Files         int       `json:"files"`
		NewFileBytes  *int64    `json:"newFileBytes"`
		FilesTarBytes *int64    `json:"filesTarBytes"`
	}
	if raw, err := os.ReadFile(filepath.Join(path, "manifest.json")); err == nil { //nolint:gosec // a file of a backup named by nameRe
		if json.Unmarshal(raw, &m) == nil {
			if !m.CreatedAt.IsZero() {
				out.CreatedAt = m.CreatedAt
			}
			out.Files = m.Files
		}
	}
	// The uploaded files (the files folder): the manifest says what they
	// take, so listing backups does not read hundreds of thousands of files.
	if m.FilesTarBytes != nil && m.NewFileBytes != nil {
		out.SizeBytes += *m.FilesTarBytes
		out.DiskBytes += *m.NewFileBytes
		return out, nil
	}
	files, err := fileParts(path)
	if err != nil {
		return Info{}, err
	}
	for _, f := range files {
		out.SizeBytes += entrySize(name+"/"+f.name, f.size)
		out.DiskBytes += f.size
	}
	return out, nil
}

// part is one file of a backup's folder.
type part struct {
	name string
	size int64
	mod  time.Time
}

// parts lists a backup folder's files, in a fixed order.
func parts(path string) ([]part, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var out []part
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, part{name: e.Name(), size: info.Size(), mod: info.ModTime()})
	}
	slices.SortFunc(out, func(a, b part) int { return compareNames(a.name, b.name) })
	return out, nil
}

// tarSize is the exact size of the .tar WriteTar writes: a 512-byte header
// and the content padded to 512 bytes for each file (names are short, so no
// extra headers), the folder's own header, and two empty blocks at the end.
func tarSize(name string, parts []part) int64 {
	size := int64(512) // the folder
	for _, p := range parts {
		size += entrySize(name+"/"+p.name, p.size)
	}
	return size + 1024
}

// Size returns the size of a backup's .tar download.
func Size(dir, name string) (int64, error) {
	info, err := Stat(dir, name)
	return info.SizeBytes, err
}

// WriteTar writes a backup's folder as a .tar to w: the folder and its files,
// so unpacking it gives the folder back, ready to restore. Exactly Size bytes.
func WriteTar(w io.Writer, dir, name string) error {
	if !nameRe.MatchString(name) {
		return ErrNotFound
	}
	path := filepath.Join(dir, name)
	files, err := parts(path)
	if err != nil {
		return err
	}
	uploaded, err := fileParts(path)
	if err != nil {
		return err
	}
	files = append(files, uploaded...)
	tw := tar.NewWriter(w)
	folder := &tar.Header{Name: name + "/", Mode: 0o700, Typeflag: tar.TypeDir, Format: tar.FormatGNU, ModTime: time.Now().Truncate(time.Second)}
	if err := tw.WriteHeader(folder); err != nil {
		return err
	}
	for _, p := range files {
		if err := tw.WriteHeader(FileHeader(name, p.name, p.size, p.mod)); err != nil {
			return err
		}
		f, err := os.Open(filepath.Join(path, filepath.FromSlash(p.name))) //nolint:gosec // a file of a backup named by nameRe
		if err != nil {
			return err
		}
		n, err := io.Copy(tw, f)
		_ = f.Close()
		if err != nil {
			return err
		}
		if n != p.size {
			return fmt.Errorf("backup: %s changed while it was being sent", p.name)
		}
	}
	return tw.Close()
}

// FileHeader is the .tar header of one of a backup's files: GNU format, one
// 512-byte block whatever the size (USTAR stops at 8 GiB; a backup's files
// archive is far larger with many CVs).
func FileHeader(backupName, file string, size int64, mod time.Time) *tar.Header {
	return &tar.Header{Name: backupName + "/" + file, Mode: 0o600, Size: size, ModTime: mod.Truncate(time.Second), Typeflag: tar.TypeReg, Format: tar.FormatGNU}
}

// Own gives the backups folder, every backup in it and their files to a
// user and group (HP_BACKUP_OWNER), keeping them private to that user. The
// backup service runs as root in Docker and the API as another user: without
// this the API could not list, send or delete them.
func Own(dir string, uid, gid int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	errs := []error{os.Chown(dir, uid, gid)}
	for _, e := range entries {
		if !e.IsDir() || !nameRe.MatchString(e.Name()) {
			continue
		}
		errs = append(errs, filepath.WalkDir(filepath.Join(dir, e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Type().IsRegular() { // its folders and files; links are left alone
				return os.Chown(p, uid, gid)
			}
			return nil
		}))
	}
	return errors.Join(errs...)
}

// Delete removes one backup from dir.
func Delete(dir, name string) error {
	if !nameRe.MatchString(name) {
		return ErrNotFound
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	return os.RemoveAll(path)
}
