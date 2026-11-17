//go:build !windows

package main

import "os"

// Unlink immediately: only the open handle can access the staging file, and
// the OS reclaims it even if the process crashes before deferred cleanup.
func createArchiveFile() (*os.File, error) {
	f, err := os.CreateTemp("", "hiringplug-ja-export-*.zip")
	if err != nil {
		return nil, err
	}
	if err := os.Remove(f.Name()); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
