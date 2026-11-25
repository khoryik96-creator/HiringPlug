package backup

import (
	"errors"
	"strconv"

	"golang.org/x/sys/unix"
)

// openLeaf opens a file of the folder dirfd as itself, never through a
// link (O_PATH|O_NOFOLLOW opens a link as the link, which walk then skips),
// whatever its permissions: changing its owner or mode needs no read access.
func openLeaf(dirfd int, name string) (int, error) {
	return unix.Openat(dirfd, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}

// chownLeaf changes the owner of the file openLeaf opened.
func chownLeaf(fd, uid, gid int) error {
	return unix.Fchownat(fd, "", uid, gid, unix.AT_EMPTY_PATH)
}

// chmodLeaf changes the mode of the file openLeaf opened: fchmodat2 (Linux
// 6.6+), else through its /proc/self/fd entry, which is that open file
// (never a name resolved again).
func chmodLeaf(fd int, mode uint32) error {
	err := unix.Fchmodat(fd, "", mode, unix.AT_EMPTY_PATH)
	if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		err = unix.Chmod("/proc/self/fd/"+strconv.Itoa(fd), mode)
	}
	return err
}
