package app_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The one-click update scripts (repository root) run on people's own
// computers, where nothing else tests them.
func TestUpdateScriptsSuitTheirSystem(t *testing.T) {
	bat, err := os.ReadFile("../../../update-hiringplug.bat")
	require.NoError(t, err)
	win := string(bat)
	// Windows has no /dev/null: a redirect there fails with "The system cannot
	// find the path specified" and the command does not run.
	assert.NotContains(t, win, "/dev/null", "use >nul in the Windows script")
	assert.Equal(t, strings.Count(win, "\n"), strings.Count(win, "\r\n"), "the Windows script needs CRLF line endings")
	// "docker info" can succeed while the engine is stopped.
	assert.NotContains(t, win, "docker info >", "check the engine with docker ps")

	sh, err := os.ReadFile("../../../update-hiringplug.command")
	require.NoError(t, err)
	assert.NotContains(t, string(sh), "\r\n", "the macOS script needs LF line endings")
	assert.NotContains(t, string(sh), "docker info >", "check the engine with docker ps")
	info, err := os.Stat("../../../update-hiringplug.command")
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o100, "the macOS script must be executable to double-click it")
}
