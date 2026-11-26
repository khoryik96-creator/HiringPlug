package buildinfo

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInDevelopmentTheVersionIsTheCheckoutsVersionFile(t *testing.T) {
	want, err := os.ReadFile("../../../../VERSION")
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(want)), Version())
}
