//go:build !unix

package alerts

import "errors"

// diskSpace is not checked off Unix (the server runs on Linux).
func diskSpace(string) (free, total uint64, err error) {
	return 0, 0, errors.New("disk space is only checked on Linux")
}
