package backup_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/backup"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestFailedRestorePreservesDatabaseAndFiles(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	testutil.Current.NewTenant(t)
	source, dest, root := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "old.pdf"), []byte("synthetic archived file"), 0o600))
	made, err := backup.Run(ctx, testutil.Current.Admin, backup.Options{
		AdminURL: testutil.Current.AdminURL, FilesDir: source, Dir: root, Keep: 1,
	})
	require.NoError(t, err)
	valid := filepath.Join(t.TempDir(), "valid-files")
	require.NoError(t, os.Rename(filepath.Join(made, "files"), valid))
	require.NoError(t, os.WriteFile(filepath.Join(made, "files.tar.gz"), []byte("invalid archive fixture"), 0o600))
	marker := filepath.Join(dest, "current.pdf")
	require.NoError(t, os.WriteFile(marker, []byte("synthetic existing file"), 0o600))
	target := freshDatabase(t)
	err = backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: root}, filepath.Base(made))
	require.Error(t, err)
	require.FileExists(t, marker)
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var absent bool
	require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass('public.tenant') IS NULL").Scan(&absent))
	require.True(t, absent)
	// A valid tar with a damaged gzip trailer also fails before installation.
	var packed bytes.Buffer
	gz := gzip.NewWriter(&packed)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "old.pdf", Size: 1, Mode: 0o600}))
	_, err = tw.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	raw := packed.Bytes()
	raw[len(raw)-8] ^= 1
	require.NoError(t, os.WriteFile(filepath.Join(made, "files.tar.gz"), raw, 0o600))
	require.ErrorContains(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: root}, filepath.Base(made)), "checksum")
	require.FileExists(t, marker)
	// Installable files paired with a broken database dump must roll back too.
	require.NoError(t, os.Rename(valid, filepath.Join(made, "files")))
	require.NoError(t, os.WriteFile(filepath.Join(made, "database.dump"), []byte("invalid dump"), 0o600))
	require.Error(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: dest, Dir: root}, filepath.Base(made)))
	require.FileExists(t, marker)
	require.NoFileExists(t, filepath.Join(dest, "old.pdf"))
}

// A backup made before an update is put back over the updated database: what
// the update added (here a table referring to tenant, and a function) goes,
// for `hp bootstrap` to make again; it does not stop the restore.
func TestARestoreOverANewerDatabaseReplacesIt(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	root := t.TempDir()
	made, err := backup.Run(ctx, testutil.Current.Admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: root, Keep: 1})
	require.NoError(t, err)
	target := freshDatabase(t)
	o := backup.Options{AdminURL: target, FilesDir: t.TempDir(), Dir: root}
	require.NoError(t, backup.Restore(ctx, o, filepath.Base(made)))

	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE newer_thing (tenant_id text NOT NULL REFERENCES tenant (id));
		INSERT INTO newer_thing VALUES ('`+tenant.ID+`');
		CREATE FUNCTION newer_count() RETURNS bigint LANGUAGE sql AS 'SELECT count(*) FROM newer_thing';`)
	require.NoError(t, err)
	pool.Close()

	require.NoError(t, backup.Restore(ctx, o, filepath.Base(made)), "the newer table no longer stops the restore")
	pool, err = database.Connect(ctx, target)
	require.NoError(t, err)
	var tenants int
	var table, fn *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM tenant WHERE id = $1),
		to_regclass('public.newer_thing')::text, to_regprocedure('public.newer_count()')::text`, tenant.ID).Scan(&tenants, &table, &fn))
	assert.Equal(t, 1, tenants)
	assert.Nil(t, table, "what the update added is gone (bootstrap makes it again)")
	assert.Nil(t, fn)
}

// A backup leaves out what search by meaning read (numbers its read-through
// reads again after a restore): tens of KB per candidate otherwise.
func TestABackupLeavesOutWhatMeaningReadsAgain(t *testing.T) {
	requireTools(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t)
	_, err := testutil.Current.Admin.Exec(ctx, `INSERT INTO meaning_query (tenant_id, text_hash, body, status)
		VALUES ($1, repeat('b', 64), 'accountant', 'ready')`, tenant.ID)
	require.NoError(t, err)
	root := t.TempDir()
	made, err := backup.Run(ctx, testutil.Current.Admin, backup.Options{AdminURL: testutil.Current.AdminURL, FilesDir: t.TempDir(), Dir: root, Keep: 1})
	require.NoError(t, err)
	target := freshDatabase(t)
	require.NoError(t, backup.Restore(ctx, backup.Options{AdminURL: target, FilesDir: t.TempDir(), Dir: root}, filepath.Base(made)))
	pool, err := database.Connect(ctx, target)
	require.NoError(t, err)
	defer pool.Close()
	var tenants, queries int
	require.NoError(t, pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM tenant WHERE id = $1), (SELECT count(*) FROM meaning_query)`, tenant.ID).Scan(&tenants, &queries))
	assert.Equal(t, 1, tenants)
	assert.Zero(t, queries)
}
