//go:build unix && !linux

package backup

import "golang.org/x/sys/unix"

// openLeaf opens a file of the folder dirfd, never through a link (O_NOFOLLOW).
// Without Linux's O_PATH it needs read access (root, or the file's owner).
func openLeaf(dirfd int, name string) (int, error) {
	return unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
}

func chownLeaf(fd, uid, gid int) error { return unix.Fchown(fd, uid, gid) }

func chmodLeaf(fd int, mode uint32) error { return unix.Fchmod(fd, mode) }
