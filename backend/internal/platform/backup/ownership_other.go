//go:build !unix

package backup

import (
	"context"
	"errors"
	"io/fs"
)

// errNotSupported: file owners are only handled on Unix (the server runs on Linux).
var errNotSupported = errors.New("file owners can only be checked on Linux and other Unix systems")

func walk(string, func(node) error) error { return errNotSupported }

func (node) chown(Owner) error { return errNotSupported }

func (node) chmod(fs.FileMode) error { return errNotSupported }

func openAs(context.Context, string, Owner) error { return errNotSupported }
