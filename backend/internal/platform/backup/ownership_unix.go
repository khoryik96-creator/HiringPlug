//go:build unix

package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// beforeOpen, when set (tests), runs just before walk opens name in dir:
// where a process racing the walk would swap it for a link.
var beforeOpen func(dir, name string)

// walk visits root, then every folder and regular file under it (the
// restore's staging folder left out), each open while visit runs. It never
// follows a link: root and each folder are opened with O_NOFOLLOW, each
// name relative to its folder's open descriptor (never by a path that
// could be resolved again), and a name that is a link, or becomes one
// between being listed and opened, is skipped. As root on a live files
// folder (hp repair-files) nothing outside it is ever changed.
func walk(root string, visit func(node) error) error {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return &fs.PathError{Op: "open", Path: root, Err: err}
	}
	return walkDir(fd, root, "", visit)
}

func walkDir(fd int, abs, rel string, visit func(node) error) error {
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return &fs.PathError{Op: "stat", Path: abs, Err: err}
	}
	if err := visit(nodeOf(fd, rel, true, &st)); err != nil {
		return err
	}
	names, err := readNames(fd, abs)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); errors.Is(err, unix.ENOENT) {
			continue // gone meanwhile
		} else if err != nil {
			return &fs.PathError{Op: "stat", Path: filepath.Join(abs, name), Err: err}
		}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			if name == stageName {
				continue
			}
			if beforeOpen != nil {
				beforeOpen(abs, name)
			}
			child, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if swapped(err) {
				continue
			} else if err != nil {
				return &fs.PathError{Op: "open", Path: filepath.Join(abs, name), Err: err}
			}
			if err := walkDir(child, filepath.Join(abs, name), path.Join(rel, name), visit); err != nil {
				return err
			}
		case unix.S_IFREG:
			if beforeOpen != nil {
				beforeOpen(abs, name)
			}
			if err := visitFile(fd, abs, path.Join(rel, name), name, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

// visitFile opens a regular file of the folder fd (openLeaf: never through
// a link) and visits it; one that is no longer a regular file is skipped.
func visitFile(dirfd int, abs, rel, name string, visit func(node) error) error {
	fd, err := openLeaf(dirfd, name)
	if swapped(err) {
		return nil
	} else if err != nil {
		return &fs.PathError{Op: "open", Path: filepath.Join(abs, name), Err: err}
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return &fs.PathError{Op: "stat", Path: filepath.Join(abs, name), Err: err}
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG { // a link (O_PATH opens the link itself), or swapped
		return nil
	}
	return visit(nodeOf(fd, rel, false, &st))
}

// swapped reports an open that failed because the name is no longer what
// was listed: gone, or a link now (never followed).
func swapped(err error) bool {
	return errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR)
}

func nodeOf(fd int, rel string, dir bool, st *unix.Stat_t) node {
	return node{fd: fd, rel: rel, dir: dir, uid: int(st.Uid), gid: int(st.Gid), perm: fs.FileMode(st.Mode & 0o777),
		nlink: uint64(st.Nlink)} //nolint:unconvert // its type differs by platform
}

// readNames lists the folder fd (through a copy of the descriptor, so fd
// stays open for the names' own opens).
func readNames(fd int, abs string) ([]string, error) {
	dup, err := unix.Dup(fd)
	if err != nil {
		return nil, &fs.PathError{Op: "dup", Path: abs, Err: err}
	}
	f := os.NewFile(uintptr(dup), abs)
	defer func() { _ = f.Close() }()
	return f.Readdirnames(-1)
}

func (n node) chown(o Owner) error {
	if n.dir {
		return n.err("chown", unix.Fchown(n.fd, o.UID, o.GID))
	}
	return n.err("chown", chownLeaf(n.fd, o.UID, o.GID))
}

func (n node) chmod(perm fs.FileMode) error {
	if n.dir {
		return n.err("chmod", unix.Fchmod(n.fd, uint32(perm)))
	}
	return n.err("chmod", chmodLeaf(n.fd, uint32(perm)))
}

func (n node) err(op string, err error) error {
	if err == nil {
		return nil
	}
	return &fs.PathError{Op: op, Path: n.rel, Err: err}
}

// openAs opens path for reading as o: itself when this process runs as o,
// otherwise (root) in a child process running as o, which also has to get
// through every folder on the way, as the API does. The child gets no
// supplementary groups (only o's group: root's would let it open what o
// cannot) and no environment (no secrets).
func openAs(ctx context.Context, path string, o Owner) error {
	if os.Geteuid() == o.UID {
		f, err := os.Open(path) //nolint:gosec // a file of the files folder, found by walking it
		if err != nil {
			return err
		}
		_, err = io.CopyN(io.Discard, f, 1)
		_ = f.Close()
		if errors.Is(err, io.EOF) {
			err = nil
		}
		return err
	}
	head, err := exec.LookPath("head")
	if err != nil {
		return fmt.Errorf("cannot open files as %s: %w", o, err)
	}
	cmd := exec.CommandContext(ctx, head, "-c", "1", "--", path) //nolint:gosec // head, on a file found by walking the files folder
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	cmd.Stdout = io.Discard
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: uint32(o.UID), Gid: uint32(o.GID), Groups: []uint32{}, //nolint:gosec // ids parsed as non-negative
	}}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
