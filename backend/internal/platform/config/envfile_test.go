package config

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every setting the code reads must be documented in .env.example, so a new
// setting can never be a surprise on someone else's machine or server.
func TestEveryVariableIsInEnvExample(t *testing.T) {
	example, err := os.ReadFile("../../../../.env.example")
	require.NoError(t, err)
	sources := []string{"config.go", "../../../cmd/hp/main.go", "../../../../ai/src/hiringplug_ai/config.py"}
	keyRe := regexp.MustCompile(`"(HP_[A-Z0-9_]+)"`)
	for _, src := range sources {
		code, err := os.ReadFile(src) //nolint:gosec // this repository's own source
		require.NoError(t, err)
		for _, m := range keyRe.FindAllStringSubmatch(string(code), -1) {
			assert.Regexp(t, `(?m)^`+m[1]+`=`, string(example), "%s reads %s; add it to .env.example", src, m[1])
		}
	}
}
