package app_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// backfillStatement is the statement of the AI use migration that fills the
// ledger from a feature's own records.
func backfillStatement(t *testing.T, feature string) string {
	t.Helper()
	raw, err := os.ReadFile("../../db/migrations/00196_ai_usage.sql")
	require.NoError(t, err)
	up, _, _ := strings.Cut(string(raw), "-- +goose Down")
	for _, stmt := range strings.Split(up, ";\n") {
		if strings.Contains(stmt, "INSERT INTO ai_use") && strings.Contains(stmt, "'"+feature+"'") {
			return stmt
		}
	}
	t.Fatalf("no backfill of %s", feature)
	return ""
}

// A formatted CV counted twice before the update (the same result delivered
// again) is one run in the ledger, under the run key used from now on, so a
// result delivered again after the update adds nothing.
func TestTheLedgerBackfillCountsAFormattedCVOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tx, err := testutil.Current.Admin.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	// Temporary tables stand in for the real ones in this transaction only.
	for _, stmt := range []string{
		`CREATE TEMP TABLE ai_use (LIKE public.ai_use INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING INDEXES) ON COMMIT DROP`,
		`CREATE TEMP TABLE studio_cv (id text, tenant_id text, created_by text) ON COMMIT DROP`,
		`CREATE TEMP TABLE studio_cv_ai_use (tenant_id text, studio_cv_id text, tokens integer, created_at timestamptz) ON COMMIT DROP`,
	} {
		_, err := tx.Exec(ctx, stmt)
		require.NoError(t, err)
	}
	first := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	_, err = tx.Exec(ctx, `INSERT INTO studio_cv VALUES ('scv_x', 'ten_x', 'usr_x'), ('scv_y', 'ten_x', 'system')`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO studio_cv_ai_use VALUES ('ten_x', 'scv_x', 4200, $1), ('ten_x', 'scv_x', 4200, $2),
		('ten_x', 'scv_y', 900, $1), ('ten_x', 'scv_y', 0, $2)`, first, first.Add(time.Minute))
	require.NoError(t, err)

	_, err = tx.Exec(ctx, backfillStatement(t, "cv_studio"))
	require.NoError(t, err)
	type row struct {
		source, runKey string
		user           *string
		input          int64
		at             time.Time
	}
	rows, err := tx.Query(ctx, `SELECT source_id, run_key, user_id, input_tokens, used_at FROM ai_use ORDER BY source_id`)
	require.NoError(t, err)
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.source, &r.runKey, &r.user, &r.input, &r.at))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	user := "usr_x"
	require.Len(t, got, 2, "one run per formatted CV")
	assert.Equal(t, row{"scv_x", "", &user, 4200, got[0].at}, got[0], "counted once, under the key used from now on")
	assert.True(t, first.Equal(got[0].at), "when it was first counted")
	assert.Equal(t, row{"scv_y", "", nil, 900, got[1].at}, got[1], "automatic work has no user")
}
