package main

// Every subcommand's flags and names, the same way: flags anywhere, before
// or after the names (`hp restore NAME -yes` and `hp restore -yes NAME`
// alike; Go's flag package alone stops at the first name and silently
// leaves the flags after it unread), a wrong number of names refused with
// the command's usage, and -h printing it (exit 0).

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// command is one subcommand's usage: its name, what follows it, and how
// many names (positional arguments) it takes.
type command struct {
	name     string
	synopsis string // e.g. "-yes [-resume-outbound] hiringplug-YYYYMMDD-HHMMSS"
	min, max int
	out      io.Writer // where -h and mistakes print the usage (stderr when nil)
}

// flags is the command's flag set; Parse prints the usage on mistakes.
func (c command) flags() *flag.FlagSet {
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	out := c.out
	if out == nil {
		out = os.Stderr
	}
	fs.SetOutput(out)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(out, "usage: hp %s %s\n", c.name, c.synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parse reads args into fs, flags anywhere among the names ("--" ends the
// flags), and returns the names, checking how many there are.
func (c command) parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var names []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err // flag.ErrHelp for -h: the usage is printed
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		if used := len(args) - len(rest); used > 0 && args[used-1] == "--" {
			names = append(names, rest...)
			break
		}
		names = append(names, rest[0])
		args = rest[1:]
	}
	if len(names) < c.min || len(names) > c.max {
		return nil, c.wrongCount(names)
	}
	return names, nil
}

func (c command) wrongCount(names []string) error {
	want := "no names"
	switch {
	case c.min == c.max && c.min == 1:
		want = "exactly one name"
	case c.min == 0 && c.max == 1:
		want = "at most one name"
	case c.max > 0:
		want = fmt.Sprintf("%d to %d names", c.min, c.max)
	}
	got := "none"
	if len(names) > 0 {
		got = fmt.Sprintf("%d (%s)", len(names), strings.Join(names, " "))
	}
	return fmt.Errorf("hp %s takes %s, got %s; usage: hp %s %s", c.name, want, got, c.name, c.synopsis)
}

// isHelp reports whether err is -h being asked for (not a failure).
func isHelp(err error) bool { return errors.Is(err, flag.ErrHelp) }
