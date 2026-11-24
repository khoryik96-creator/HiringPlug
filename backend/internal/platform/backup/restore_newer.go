package backup

// Putting a backup back over a database that an update made newer: what the
// update added (a table, a function, an extension) is not in the backup, and
// would stop the backup's own objects from being replaced (a new table that
// refers to candidate, say). So the restore drops those first, in the same
// transaction as the rest; `hp bootstrap` then brings the database up to
// date again, making them anew. Should anything fail, nothing has changed.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jackc/pgx/v5"
)

// restoreDatabase puts the dump back in one transaction.
func restoreDatabase(ctx context.Context, o Options, major int, dump string) error {
	pgRestore, err := o.tools().tool("pg_restore", major)
	if err != nil {
		return err
	}
	psql, err := o.tools().tool("psql", major)
	if err != nil {
		return err
	}
	toc, err := exec.CommandContext(ctx, pgRestore, "--list", dump).CombinedOutput() //nolint:gosec // configured PostgreSQL tool
	if err != nil {
		return fmt.Errorf("pg_restore --list: %w: %s", err, toc)
	}
	drops, err := newerObjects(ctx, o.AdminURL, dumped(string(toc)))
	if err != nil {
		return err
	}
	// The backup as SQL, written out in full before any of it runs.
	script, err := os.CreateTemp(o.Dir, ".restore-*.sql")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(script.Name()) }()
	if err := script.Close(); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, pgRestore, "--clean", "--if-exists", "--file", script.Name(), dump).CombinedOutput(); err != nil { //nolint:gosec // configured PostgreSQL tool
		return fmt.Errorf("pg_restore: %w: %s", err, hidePassword(string(out), o.AdminURL))
	}
	args := []string{"--no-psqlrc", "--quiet", "--single-transaction", "--set", "ON_ERROR_STOP=1", "--dbname", o.AdminURL}
	if len(drops) > 0 {
		args = append(args, "--command", strings.Join(drops, "\n"))
	}
	args = append(args, "--file", script.Name())
	if out, err := exec.CommandContext(ctx, psql, args...).CombinedOutput(); err != nil { //nolint:gosec // configured PostgreSQL tool
		return fmt.Errorf("restoring the database: %w: %s", err, hidePassword(string(out), o.AdminURL))
	}
	return nil
}

// dumped reads a backup's list of contents (pg_restore --list) into the
// objects it holds, as "TABLE candidate", "FUNCTION app_current_tenant",
// "EXTENSION vector".
func dumped(toc string) map[string]bool {
	kinds := []struct{ prefix, kind string }{
		{"MATERIALIZED VIEW public ", "VIEW"}, {"TABLE public ", "TABLE"}, {"VIEW public ", "VIEW"},
		{"FUNCTION public ", "FUNCTION"}, {"EXTENSION - ", "EXTENSION"}, {"TYPE public ", "TYPE"},
		{"DOMAIN public ", "TYPE"}, {"SEQUENCE public ", "SEQUENCE"},
	}
	out := map[string]bool{}
	for _, line := range strings.Split(toc, "\n") {
		_, entry, ok := strings.Cut(line, "; ")
		if !ok || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.SplitN(entry, " ", 3) // catalog id, object id, the rest
		if len(fields) < 3 {
			continue
		}
		for _, k := range kinds {
			if rest, found := strings.CutPrefix(fields[2], k.prefix); found {
				name, _, _ := strings.Cut(rest, " ")
				if k.kind == "FUNCTION" {
					name, _, _ = strings.Cut(rest, "(")
				}
				out[k.kind+" "+name] = true
				break
			}
		}
	}
	return out
}

// newerObjects returns the statements that drop what the database has and
// the backup does not (an update since): tables, views, sequences,
// functions, types and extensions of the database's own (not an
// extension's).
func newerObjects(ctx context.Context, url string, inDump map[string]bool) ([]string, error) {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close(ctx) }()
	rows, err := conn.Query(ctx, `
		WITH own AS (SELECT n.oid FROM pg_namespace n WHERE n.nspname = 'public'),
		     ext AS (SELECT objid FROM pg_depend WHERE deptype = 'e')
		SELECT 'EXTENSION', extname, format('DROP EXTENSION IF EXISTS %I CASCADE;', extname) FROM pg_extension WHERE extname <> 'plpgsql'
		UNION ALL
		SELECT CASE WHEN c.relkind IN ('v', 'm') THEN 'VIEW' WHEN c.relkind = 'S' THEN 'SEQUENCE' ELSE 'TABLE' END, c.relname,
		       format(CASE c.relkind WHEN 'v' THEN 'DROP VIEW IF EXISTS public.%I CASCADE;' WHEN 'm' THEN 'DROP MATERIALIZED VIEW IF EXISTS public.%I CASCADE;'
		                             WHEN 'S' THEN 'DROP SEQUENCE IF EXISTS public.%I CASCADE;' ELSE 'DROP TABLE IF EXISTS public.%I CASCADE;' END, c.relname)
		  FROM pg_class c WHERE c.relnamespace IN (SELECT oid FROM own) AND c.relkind IN ('r', 'p', 'v', 'm', 'S')
		   AND c.oid NOT IN (SELECT objid FROM ext)
		UNION ALL
		SELECT 'FUNCTION', p.proname, format('DROP FUNCTION IF EXISTS %s CASCADE;', p.oid::regprocedure)
		  FROM pg_proc p WHERE p.pronamespace IN (SELECT oid FROM own) AND p.oid NOT IN (SELECT objid FROM ext)
		UNION ALL
		SELECT 'TYPE', t.typname, format('DROP TYPE IF EXISTS public.%I CASCADE;', t.typname)
		  FROM pg_type t WHERE t.typnamespace IN (SELECT oid FROM own) AND t.typtype IN ('e', 'd') AND t.oid NOT IN (SELECT objid FROM ext)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var drops []string
	for rows.Next() {
		var kind, name, drop string
		if err := rows.Scan(&kind, &name, &drop); err != nil {
			return nil, err
		}
		if !inDump[kind+" "+name] {
			drops = append(drops, drop)
		}
	}
	return drops, rows.Err()
}
