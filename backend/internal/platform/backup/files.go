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
	out := Info{Name: name, SizeBytes: tarSize(parts)}
	out.CreatedAt, _ = time.Parse("20060102-150405", name[len("hiringplug-"):])
	if raw, err := os.ReadFile(filepath.Join(path, "manifest.json")); err == nil { //nolint:gosec // a file of a backup named by nameRe
		var m struct {
			CreatedAt time.Time `json:"createdAt"`
			Files     int       `json:"files"`
		}
		if json.Unmarshal(raw, &m) == nil {
			if !m.CreatedAt.IsZero() {
				out.CreatedAt = m.CreatedAt
			}
			out.Files = m.Files
		}
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
func tarSize(parts []part) int64 {
	size := int64(512) // the folder
	for _, p := range parts {
		size += 512 + (p.size+511)/512*512
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
	tw := tar.NewWriter(w)
	folder := &tar.Header{Name: name + "/", Mode: 0o700, Typeflag: tar.TypeDir, Format: tar.FormatUSTAR, ModTime: time.Now().Truncate(time.Second)}
	if err := tw.WriteHeader(folder); err != nil {
		return err
	}
	for _, p := range files {
		hdr := &tar.Header{Name: name + "/" + p.name, Mode: 0o600, Size: p.size, ModTime: p.mod.Truncate(time.Second), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(filepath.Join(path, p.name)) //nolint:gosec // a file of a backup named by nameRe
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
