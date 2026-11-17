package database

// Searches and row-level security.
//
// Every tenant table has row-level security. PostgreSQL applies a policy's
// own checks before any condition that is not marked LEAKPROOF, and so it
// will not use an index for such a condition: LIKE on the trigram indexes,
// @@ on the text-search indexes and @> on the tag indexes all became full
// scans, reading every CV for every boolean search (seconds with half a
// million CVs). Marking the operators' functions LEAKPROOF lets the indexes
// work again; the policies still decide which rows anyone sees.
//
// LEAKPROOF promises a function reveals nothing about its arguments except
// through its result. These raise no errors that depend on the row (a bad
// LIKE pattern is the query's, not the row's), and only the app's own fixed
// queries run as the app roles, so nobody can probe with them anyway.
//
// It is set after every migration and restore, not in a migration: a new
// version of a function (CREATE OR REPLACE) or of pg_trgm, a restore that
// recreates the extension, or a major upgrade would quietly drop the mark.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

// SearchFunctions are the functions behind the operators the search indexes
// serve (LIKE, ILIKE, @@, @>, &&, the trigram similarity operators) and the
// app's own functions inside indexed expressions.
var SearchFunctions = []string{
	"textlike(text,text)", "texticlike(text,text)", "ts_match_vq(tsvector,tsquery)",
	"arraycontains(anyarray,anyarray)", "arrayoverlap(anyarray,anyarray)",
	"similarity_op(text,text)", "word_similarity_op(text,text)", "word_similarity_commutator_op(text,text)",
	"textcat(text,text)", "app_lower_array(text[])", "app_search_doc(text[])", "app_normalize_text(text)",
}

const markLeakproof = `
DO $$
DECLARE f text; p regprocedure;
BEGIN
  FOREACH f IN ARRAY ARRAY['%s'] LOOP
    p := to_regprocedure(f);
    IF p IS NOT NULL AND NOT (SELECT proleakproof FROM pg_proc WHERE oid = p) THEN
      EXECUTE format('ALTER FUNCTION %%s LEAKPROOF', p);
    END IF;
  END LOOP;
END $$`

// AllowSearchIndexes marks the search functions LEAKPROOF (as the admin, a
// superuser: only a superuser may). Without that right it warns on w and
// carries on: searches work, only slower.
func AllowSearchIndexes(ctx context.Context, adminURL string, w io.Writer) error {
	cfg, err := stdlibConfig(adminURL)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	return allowSearchIndexes(ctx, db, w)
}

func allowSearchIndexes(ctx context.Context, db *sql.DB, w io.Writer) error {
	if w == nil {
		w = os.Stderr
	}
	// A DO block takes no parameters: the (constant) list goes in as text.
	_, err := db.ExecContext(ctx, fmt.Sprintf(markLeakproof, strings.Join(SearchFunctions, "','")))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42501" { // insufficient_privilege
		_, _ = fmt.Fprintln(w, "warning: the database admin is not a superuser, so searches cannot use their indexes and will be slow with many records")
		return nil
	}
	if err != nil {
		return fmt.Errorf("allowing the search indexes: %w", err)
	}
	return nil
}
