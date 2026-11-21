package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate applies every pending migration in migrations (db/migrations,
// embedded into the binary). It must run as the admin/owner role, never the
// restricted app role. Then the searches' indexes are allowed (see
// AllowSearchIndexes).
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
	statuses, err := provider.Status(ctx)
	if err != nil {
		return fmt.Errorf("migrate: reading migration status: %w", err)
	}
	if isApolloPreviewUpgrade(statuses) {
		provider, err = goose.NewProvider(goose.DialectPostgres, db, migrations, goose.WithAllowOutofOrder(true))
		if err != nil {
			return fmt.Errorf("migrate: preparing Apollo preview upgrade: %w", err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return allowSearchIndexes(ctx, db, nil)
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

// The Apollo preview shipped 157 while the earlier Leads schema was still in
// a separate branch. Permit that one independent migration for preview users
// while keeping the usual order check for all other missing migrations.
func isApolloPreviewUpgrade(statuses []*goose.MigrationStatus) bool {
	const leadsVersion, apolloVersion = 156, 157
	var latest int64
	var apolloApplied, leadsPending bool
	for _, status := range statuses {
		if status.State == goose.StateApplied {
			latest = max(latest, status.Source.Version)
			apolloApplied = apolloApplied || (status.Source.Version == apolloVersion && path.Base(status.Source.Path) == "00157_apollo_hr.sql")
		} else if status.Source.Version == leadsVersion && path.Base(status.Source.Path) == "00156_leads_workspace.sql" {
			leadsPending = true
		}
	}
	if !apolloApplied || !leadsPending {
		return false
	}
	for _, status := range statuses {
		if status.State == goose.StatePending && status.Source.Version < latest && status.Source.Version != leadsVersion {
			return false
		}
	}
	return true
}
