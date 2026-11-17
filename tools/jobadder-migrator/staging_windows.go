package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// Windows cannot unlink a normal open file. A kernel delete-on-close handle
// gives the export the same crash cleanup guarantee as an unlinked Unix file.
func createArchiveFile() (*os.File, error) {
	f, err := os.CreateTemp("", "hiringplug-ja-export-*.zip")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_TEMPORARY|windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	return os.NewFile(uintptr(h), name), nil
}
