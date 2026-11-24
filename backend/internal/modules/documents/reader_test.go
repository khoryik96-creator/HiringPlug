package documents

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server tells a CV read by older rules by the AI worker's reading
// version: raise both together, or reviewed CVs are never read again (or
// always are).
func TestTheCVReaderIsTheAIWorkersOwn(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "ai", "src", "hiringplug_ai", "parsing", "__init__.py"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^PARSER = "([^"]+)"`).FindSubmatch(src)
	require.NotNil(t, m, `the AI worker says PARSER = "..."`)
	assert.Equal(t, string(m[1]), CVReader)
}
