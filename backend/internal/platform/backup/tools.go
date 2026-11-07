package backup

// Finding pg_dump and pg_restore of the database server's own major version:
// a newer pg_dump writes settings an older server refuses on restore (e.g.
// PostgreSQL 17's transaction_timeout on a 16 server), so its backup could
// never be put back.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// serverMajor returns the server's major version (16 for 16.4).
func serverMajor(ctx context.Context, url string) (int, error) {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return 0, fmt.Errorf("connecting to the database: %s", hidePassword(err.Error(), url))
	}
	defer func() { _ = conn.Close(ctx) }()
	var num int
	if err := conn.QueryRow(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&num); err != nil {
		return 0, err
	}
	return num / 10000, nil
}

// toolFinder looks for programs (replaced in tests).
type toolFinder struct {
	lookPath func(string) (string, error)
	exists   func(string) bool
	version  func(path string) (string, error) // e.g. "pg_dump (PostgreSQL) 17.2"
}

var systemTools = toolFinder{
	lookPath: exec.LookPath,
	exists:   func(p string) bool { _, err := os.Stat(p); return err == nil },
	version: func(p string) (string, error) {
		out, err := exec.Command(p, "--version").Output() //nolint:gosec,noctx // a PostgreSQL tool found on this machine
		return string(out), err
	},
}

var versionRe = regexp.MustCompile(`\(PostgreSQL\) (\d+)`)

// tool returns the path of name (pg_dump, pg_restore) for a server of major
// version major: Debian and Ubuntu keep each version's own in
// /usr/lib/postgresql/<major>/bin; otherwise the one on PATH, if it is of the
// same major version.
func (f toolFinder) tool(name string, major int) (string, error) {
	if p := filepath.Join("/usr/lib/postgresql", strconv.Itoa(major), "bin", name); f.exists(p) {
		return p, nil
	}
	p, err := f.lookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is not installed: install the PostgreSQL %d client tools", name, major)
	}
	v, err := f.version(p)
	if err != nil {
		return "", fmt.Errorf("%s --version: %w", p, err)
	}
	m := versionRe.FindStringSubmatch(v)
	if m == nil {
		return "", fmt.Errorf("%s: unknown version %q", p, v)
	}
	if got, _ := strconv.Atoi(m[1]); got != major {
		return "", fmt.Errorf("%s is PostgreSQL %d's but the database is PostgreSQL %d: install the PostgreSQL %d client tools (a backup made otherwise could not be put back)",
			name, got, major, major)
	}
	return p, nil
}
