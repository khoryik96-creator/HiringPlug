package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestBinPurgeCommitFailurePreservesTheCV(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Synthetic CV Owner"})
	uploaded := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "synthetic.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, uploaded.Status, "%s", uploaded.Body)
	var doc api.Document
	uploaded.JSON(&doc)
	var key string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, "SELECT storage_key FROM document WHERE id = $1", doc.Id).Scan(&key))
	f, err := a.deps.Files.Open(key)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	binned := a.bin(token, map[string]any{"kind": "candidate", "id": c.Id})
	require.Equal(t, http.StatusCreated, binned.Status, "%s", binned.Body)

	_, err = testutil.Current.Admin.Exec(ctx,
		"CREATE FUNCTION test_abort_bin_commit() RETURNS trigger LANGUAGE plpgsql AS $$ "+
			"BEGIN RAISE EXCEPTION 'synthetic purge commit failure'; END $$; "+
			"CREATE CONSTRAINT TRIGGER test_abort_bin_commit "+
			"AFTER DELETE ON bin_entry DEFERRABLE INITIALLY DEFERRED "+
			"FOR EACH ROW WHEN (OLD.record_id = '"+c.Id+"') EXECUTE FUNCTION test_abort_bin_commit();")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(ctx, "DROP TRIGGER IF EXISTS test_abort_bin_commit ON bin_entry; DROP FUNCTION IF EXISTS test_abort_bin_commit()")
	})
	purgeNow(t, a, tenant.ID)
	require.Len(t, a.binList(token), 1, "the transaction rolled the purge back")
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, "SELECT storage_key FROM document WHERE id = $1", doc.Id).Scan(&key))
	f, err = a.deps.Files.Open(key)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
