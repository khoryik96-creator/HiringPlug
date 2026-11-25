//go:build !unix

package backup

import (
	"context"
	"errors"
	"io/fs"
)

func ownerOf(fs.FileInfo) (uid, gid int, ok bool) { return 0, 0, false }

func links(fs.FileInfo) uint64 { return 1 }

// errNotSupported: file owners are only handled on Unix (the server runs on Linux).
var errNotSupported = errors.New("file owners can only be checked on Linux and other Unix systems")

func openAs(context.Context, string, Owner) error { return errNotSupported }
