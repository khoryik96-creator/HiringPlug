package app_test

import (
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestApolloPreviewDatabaseReceivesTheLeadsMigration(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, err := testutil.Current.Admin.Exec(ctx, "CREATE SCHEMA apollo_preview_upgrade")
	require.NoError(t, err)
	schemaURL, err := url.Parse(testutil.Current.AdminURL)
	require.NoError(t, err)
	opts := schemaURL.Query()
	opts.Set("search_path", "apollo_preview_upgrade")
	schemaURL.RawQuery = opts.Encode()

	// Small migrations isolate the version sequence. The app's normal test
	// database also starts with all the actual migrations applied.
	const emptyMigration = "-- +goose Up\nSELECT 1;"
	migrations := fstest.MapFS{"00157_apollo_hr.sql": {Data: []byte(emptyMigration)}}
	require.NoError(t, database.Migrate(ctx, schemaURL.String(), migrations))
	migrations["00156_leads_workspace.sql"] = &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE leads_upgrade_probe (id int);")}
	migrations["00158_apollo_revocation.sql"] = &fstest.MapFile{Data: []byte(emptyMigration)}
	require.NoError(t, database.Migrate(ctx, schemaURL.String(), migrations))
	require.NoError(t, database.Migrate(ctx, schemaURL.String(), migrations), "repeat updates are safe")
	var count int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `
		SELECT count(*) FROM apollo_preview_upgrade.goose_db_version
		WHERE version_id IN (156, 157, 158) AND is_applied`).Scan(&count))
	require.Equal(t, 3, count)
	_, err = testutil.Current.Admin.Exec(ctx, "INSERT INTO apollo_preview_upgrade.leads_upgrade_probe (id) VALUES (1)")
	require.NoError(t, err)
}
