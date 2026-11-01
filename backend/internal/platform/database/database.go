// Package database connects to PostgreSQL and runs every unit of work inside
// a transaction scoped to one tenant and one principal.
//
// InTx is the only way application code should touch the database. It sets
// app.tenant_id, app.user_id and app.permissions with SET LOCAL semantics, so
// row-level security limits every query to that tenant (and salary rows to
// permitted users), and nothing leaks to the next transaction on a pooled
// connection.
package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Connect opens a connection pool and checks the database is reachable.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: cannot reach the database: %w", err)
	}
	return pool, nil
}

// InTx runs fn in a transaction scoped to p. If fn returns an error (or
// panics) everything it did is rolled back.
func InTx(ctx context.Context, pool *pgxpool.Pool, p authz.Principal, fn func(pgx.Tx) error) error {
	if p.TenantID == "" {
		return errors.New("database: InTx needs a principal with a tenant")
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`SELECT set_config('app.tenant_id', $1, true), set_config('app.user_id', $2, true), set_config('app.permissions', $3, true)`,
			p.TenantID, p.UserID, p.PermissionCSV(),
		); err != nil {
			return fmt.Errorf("database: scoping transaction: %w", err)
		}
		return fn(tx)
	})
}

// CheckRestrictedRole refuses to run the app as a database role that would
// bypass row-level security: a superuser, a role with BYPASSRLS, or the owner
// of the tables. Called at startup by the API and the worker.
func CheckRestrictedRole(ctx context.Context, pool *pgxpool.Pool) error {
	var user string
	var super, bypass, ownsTables bool
	err := pool.QueryRow(ctx, `
		SELECT current_user, r.rolsuper, r.rolbypassrls,
		       EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		               WHERE n.nspname = 'public' AND c.relkind = 'r' AND pg_get_userbyid(c.relowner) = current_user)
		FROM pg_roles r WHERE r.rolname = current_user`).Scan(&user, &super, &bypass, &ownsTables)
	if err != nil {
		return fmt.Errorf("database: checking role: %w", err)
	}
	switch {
	case super:
		return fmt.Errorf("database: HP_DATABASE_URL connects as %q, a superuser, which bypasses tenant isolation; use the restricted app role", user)
	case bypass:
		return fmt.Errorf("database: role %q has BYPASSRLS, which bypasses tenant isolation; use the restricted app role", user)
	case ownsTables:
		return fmt.Errorf("database: role %q owns the tables, which bypasses tenant isolation; connect as the restricted app role, not the migration role", user)
	}
	return nil
}

// IsUniqueViolation reports whether err is a unique-constraint violation on
// the named constraint or index.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// IsNoRows reports whether err means the query returned no row.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
