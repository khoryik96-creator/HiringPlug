package blob

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheFilesFolderCheckLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s, err := New(dir, []byte("secret"), "http://x")
	require.NoError(t, err)
	require.NoError(t, s.Check())
	left, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, left)
}

func TestTheFilesFolderCheckSaysWhyItFailed(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "files")
	s, err := New(dir, []byte("secret"), "http://x")
	require.NoError(t, err)
	require.NoError(t, os.Remove(dir)) // gone: a volume not mounted, say
	err = s.Check()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot write")
}
