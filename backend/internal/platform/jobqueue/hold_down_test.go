package jobqueue_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/db"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Review of 1.47.0: undoing migration 00200 made the jobs a restore held
// pending, so they ran: emails sent and webhooks posted a second time. A
// held job is cancelled (dead) instead, and never runs.
func TestUndoingTheRestoreHoldNeverRunsWhatWasHeld(t *testing.T) {
	ctx := context.Background()
	name := testutil.Current.Name + "_hold_down"
	_, err := testutil.Current.Admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	u, err := url.Parse(testutil.Current.AdminURL)
	require.NoError(t, err)
	u.Path = "/" + name
	cfg, err := pgx.ParseConfig(u.String())
	require.NoError(t, err)
	sqlDB := stdlib.OpenDB(*cfg)
	defer func() { _ = sqlDB.Close() }()
	migrations, err := goose.NewProvider(goose.DialectPostgres, sqlDB, db.Migrations())
	require.NoError(t, err)
	_, err = migrations.UpTo(ctx, 200)
	require.NoError(t, err)

	var tenant string
	require.NoError(t, sqlDB.QueryRowContext(ctx, `INSERT INTO tenant (name) VALUES ('Acme') RETURNING id`).Scan(&tenant))
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO job (tenant_id, queue, kind, payload, status, held_at) VALUES
		($1, 'core', 'email.send', '{}', 'held', now()), ($1, 'core', 'webhook.deliver', '{}', 'pending', NULL)`, tenant)
	require.NoError(t, err)
	_, err = migrations.DownTo(ctx, 199)
	require.NoError(t, err)

	got := map[string]string{}
	rows, err := sqlDB.QueryContext(ctx, `SELECT kind, status FROM job WHERE tenant_id = $1`, tenant)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, status string
		require.NoError(t, rows.Scan(&kind, &status))
		got[kind] = status
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[string]string{"email.send": "dead", "webhook.deliver": "pending"}, got)
}
