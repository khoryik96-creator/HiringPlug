package backup

// A backup's uploaded files are a folder ("files") holding one entry per
// file. A file that has not changed since the previous backup is a hard link
// to that backup's copy, so it takes no more disk: each night only the new
// files are copied, and fourteen backups of 100 GB of CVs cost about 100 GB,
// not fourteen times that. Every backup still reads as complete on its own
// (delete any of them, download any of them, restore any of them).

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const filesFolder = "files"

// previousFiles is the files folder of the newest backup in dir that has
// one ("" when none): what the next backup shares unchanged files with.
func previousFiles(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && nameRe.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names) // the names sort by time
	for i := len(names) - 1; i >= 0; i-- {
		path := filepath.Join(dir, names[i], filesFolder)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

// linkFiles puts every regular file under src into dst: shared with prev's
// copy when it is the same there (size and time), copied otherwise. It
// returns how many files there are, how many bytes were copied (new since
// prev), and what they take in a .tar under tarPrefix.
func linkFiles(src, prev, dst, tarPrefix string) (n int, copied, tarBytes int64, err error) {
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return 0, 0, 0, err
	}
	err = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == src {
				return filepath.SkipDir // no files yet
			}
			return err
		}
		if !d.Type().IsRegular() { // folders are implied; links are never followed
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		n++
		tarBytes += entrySize(tarPrefix+"/"+filepath.ToSlash(rel), info.Size())
		if prev != "" {
			old := filepath.Join(prev, rel)
			if was, err := os.Lstat(old); err == nil && was.Mode().IsRegular() && was.Size() == info.Size() && was.ModTime().Equal(info.ModTime()) {
				if os.Link(old, target) == nil {
					return nil
				}
				// This disk cannot share files (some network and Windows
				// folders): the file is copied instead.
			}
		}
		copied += info.Size()
		return copyFile(p, target, info.ModTime())
	})
	return n, copied, tarBytes, err
}

// copyFile copies src to a new file dst, private to its owner, keeping the
// file's time (the next backup compares it to know the file is unchanged).
func copyFile(src, dst string, mod time.Time) error {
	in, err := os.Open(src) //nolint:gosec // a file under our own files or backups folder
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // a new file in a folder of ours
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Chtimes(dst, mod, mod)
}

// copyTree copies every regular file under src into dst (restoring a
// backup's files).
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return copyFile(p, target, info.ModTime())
	})
}

// fileParts lists a backup's uploaded files (its files folder), by name.
func fileParts(path string) ([]part, error) {
	var out []part
	root := filepath.Join(path, filesFolder)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == root {
				return filepath.SkipDir // a backup made before files were kept this way
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, part{name: filepath.ToSlash(rel), size: info.Size(), mod: info.ModTime()})
		return nil
	})
	slices.SortFunc(out, func(a, b part) int { return compareNames(a.name, b.name) })
	return out, err
}

// entrySize is what one file takes in the .tar: its header, its bytes
// rounded up to whole blocks, and for a name longer than a header holds
// (100 bytes) the extra entry that carries it.
func entrySize(nameInTar string, size int64) int64 {
	n := int64(512) + (size+511)/512*512
	if len(nameInTar) > 100 {
		n += 512 + (int64(len(nameInTar))+1+511)/512*512
	}
	return n
}
