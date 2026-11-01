package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate applies every pending migration in migrations (db/migrations,
// embedded into the binary). It must run as the admin/owner role, never the
// restricted app role.
func Migrate(ctx context.Context, adminURL string, migrations fs.FS) error {
	cfg, err := stdlibConfig(adminURL)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	return migrateDB(ctx, db, migrations)
}

func migrateDB(ctx context.Context, db *sql.DB, migrations fs.FS) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// PendingMigrations returns how many migrations have not been applied. The
// API refuses to start against a database that is behind the code.
func PendingMigrations(ctx context.Context, url string, migrations fs.FS) (int, error) {
	cfg, err := stdlibConfig(url)
	if err != nil {
		return 0, err
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		return 0, fmt.Errorf("migrate: %w", err)
	}
	statuses, err := provider.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate: reading migration status: %w", err)
	}
	pending := 0
	for _, s := range statuses {
		if s.State == goose.StatePending {
			pending++
		}
	}
	return pending, nil
}
