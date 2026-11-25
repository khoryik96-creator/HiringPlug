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
	"strings"
	"syscall"
)

// ownerOf is a file's user and group ids.
func ownerOf(info fs.FileInfo) (uid, gid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(st.Uid), int(st.Gid), true
}

// links is how many names a file has (more than one: hard-linked elsewhere).
func links(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink) //nolint:unconvert // its type differs by platform
	}
	return 1
}

// openAs opens path for reading as o: itself when this process runs as o,
// otherwise (root) in a child process running as o, which also has to get
// through every folder on the way, as the API does.
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
	cmd.Stdout = io.Discard
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid: uint32(o.UID), Gid: uint32(o.GID), NoSetGroups: true, //nolint:gosec // ids parsed as non-negative
	}}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
