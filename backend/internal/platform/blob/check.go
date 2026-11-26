package blob

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Check writes a small file into the storage folder, reads it back and
// removes it: the server check that the API's user may keep and open files
// there (after a restore, files owned by another user cannot be opened).
func (s *Store) Check() error {
	want := []byte("hiringplug files check")
	f, err := os.CreateTemp(s.dir, ".check-*")
	if err != nil {
		return fmt.Errorf("cannot write: %w", err)
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	_, werr := f.Write(want)
	if err := errors.Join(werr, f.Close()); err != nil {
		return fmt.Errorf("cannot write: %w", err)
	}
	got, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		return fmt.Errorf("cannot read back: %w", err)
	}
	if !bytes.Equal(got, want) {
		return errors.New("read back something else")
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("cannot remove: %w", err)
	}
	return nil
}
