package app_test

import (
	"context"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// BUG-002/003: after a restore the CVs were there but unreadable by the API
// (root's files). The workers' download said 404, so the extraction gave up
// on a file a repair would bring back, and nobody saw why. Now a file that
// is there but cannot be opened is a server error (500) everywhere, and only
// a file that is gone is "not found".
func TestAStoredFileThatCannotBeOpenedIsNotReportedMissing(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	c := a.createCandidate(token, map[string]any{"firstName": "Aisyah"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var doc api.Document
	res.JSON(&doc)
	t.Cleanup(func() {
		_, _ = testutil.Current.Admin.Exec(context.Background(), `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	})
	var link string
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT payload->>'downloadUrl' FROM job
		WHERE kind = 'document.extract_text' AND payload->>'documentId' = $1`, doc.Id).Scan(&link))
	u, err := url.Parse(link)
	require.NoError(t, err)

	stored := storedFile(t, doc.Id)
	// Unopenable even by root: a link to itself (as root, permissions would not stop the test).
	require.NoError(t, os.Remove(stored))
	require.NoError(t, os.Symlink(stored, stored))
	assert.Equal(t, http.StatusInternalServerError, a.request(http.MethodGet, u.RequestURI(), "", nil).Status,
		"the worker tries again, instead of giving up on a file that is there")
	assert.Equal(t, http.StatusInternalServerError, a.request(http.MethodGet, "/v1/documents/"+doc.Id+"/file", token, nil).Status)

	require.NoError(t, os.Remove(stored))
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, u.RequestURI(), "", nil).Status, "gone: not found")
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+doc.Id+"/file", token, nil).Status)
}

// storedFile finds where a document's file is kept in the files folder.
func storedFile(t *testing.T, documentID string) string {
	t.Helper()
	var found string
	require.NoError(t, filepath.WalkDir(filesDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == documentID {
			found = p
		}
		return err
	}))
	require.NotEmpty(t, found, "the document's file")
	return found
}
