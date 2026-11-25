package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BUG-001: `hp restore NAME -yes` failed ("usage: hp restore ... -yes"):
// Go's flags stop at the first name, so -yes after it went unread. Every
// command now takes its flags before or after its names.
func TestFlagsGoBeforeOrAfterTheNames(t *testing.T) {
	c := command{name: "restore", synopsis: "-yes NAME", min: 1, max: 1, out: &bytes.Buffer{}}
	for _, args := range [][]string{
		{"hiringplug-20261125-023050", "-yes"},
		{"-yes", "hiringplug-20261125-023050"},
		{"--yes", "hiringplug-20261125-023050"},
		{"-resume-outbound", "hiringplug-20261125-023050", "-yes=true"},
	} {
		fs := c.flags()
		yes := fs.Bool("yes", false, "")
		fs.Bool("resume-outbound", false, "")
		names, err := c.parse(fs, args)
		require.NoError(t, err, args)
		assert.Equal(t, []string{"hiringplug-20261125-023050"}, names, args)
		assert.True(t, *yes, args)
	}
	fs := c.flags()
	names, err := c.parse(fs, []string{"--", "-not-a-flag"})
	require.NoError(t, err)
	assert.Equal(t, []string{"-not-a-flag"}, names, "after --, names only")
}

func TestAWrongCommandSaysWhatItTakes(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		cmd  string
		args []string
		want string
	}{
		{"restore", nil, "hp restore takes exactly one name, got none; usage: hp restore -yes [-resume-outbound] hiringplug-YYYYMMDD-HHMMSS"},
		{"restore", []string{"hiringplug-20261125-023050", "hiringplug-20261124-023050", "-yes"}, "got 2 (hiringplug-20261125-023050 hiringplug-20261124-023050)"},
		{"restore", []string{"latest", "-yes"}, `"latest" is not a backup name`},
		{"restore", []string{"-no", "hiringplug-20261125-023050"}, "flag provided but not defined: -no"},
		{"fetch", []string{"a", "b"}, "hp fetch takes at most one name"},
		{"tenant", []string{"-name", "Acme", "stray", "-owner-email", "you@acme.test"}, "hp tenant takes no names, got 1 (stray)"},
		{"migrate", []string{"now"}, "hp migrate takes no names"},
		{"repair-files", []string{"apply"}, "hp repair-files takes no names, got 1 (apply); usage: hp repair-files [-apply]"},
		{"backups", nil, `unknown command "backups"`},
	} {
		err := run(ctx, tc.cmd, tc.args)
		require.Error(t, err, tc.cmd, tc.args)
		assert.Contains(t, err.Error(), tc.want, tc.args)
		assert.Equal(t, 1, exitCode(err, &bytes.Buffer{}), "a failure exits non-zero, so a script stops")
	}
}

// A restore never runs without -yes, in either order, and says how.
func TestRestoreRefusesWithoutYes(t *testing.T) {
	for _, args := range [][]string{{"hiringplug-20261125-023050"}, {"-resume-outbound", "hiringplug-20261125-023050"}} {
		err := run(context.Background(), "restore", args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stop the API and workers, then run hp restore -yes hiringplug-20261125-023050")
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, cmd := range []string{"restore", "fetch", "tenant", "backup", "repair-files", "migrate"} {
		err := run(context.Background(), cmd, []string{"-h"})
		require.True(t, errors.Is(err, flag.ErrHelp), cmd)
		var out bytes.Buffer
		assert.Equal(t, 0, exitCode(err, &out), cmd)
		assert.Empty(t, out.String(), "no error printed")
	}
	assert.Equal(t, 0, exitCode(run(context.Background(), "help", nil), &bytes.Buffer{}))
}
