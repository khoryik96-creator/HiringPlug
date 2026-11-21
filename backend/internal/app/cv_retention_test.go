package app_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestCVReplacementLimitsAndBin(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleAdmin, api.RoleRecruiter)
	admin, recruiter := tenant.Users[api.RoleAdmin].Token, tenant.Users[api.RoleRecruiter].Token
	candidate := a.createCandidate(recruiter, map[string]any{"firstName": "Aina"})
	other := a.createCandidate(recruiter, map[string]any{"firstName": "Siti"})
	path := "/v1/candidates/" + candidate.Id + "/documents"
	create := func(path, source string, n int, replace ...string) *response {
		content := append(bytes.Clone(minimalPDF), bytes.Repeat([]byte{'\n'}, n)...)
		return a.upload(path, recruiter, source, fmt.Sprintf("cv-%d.pdf", n), content, replace...)
	}
	document := func(r *response) api.Document {
		require.Equal(t, http.StatusCreated, r.Status, "%s", r.Body)
		var d api.Document
		r.JSON(&d)
		return d
	}
	isLimit := func(r *response) {
		require.Equal(t, http.StatusConflict, r.Status, "%s", r.Body)
		assert.Equal(t, api.ErrorCodeDOCUMENTLIMIT, r.Error().Error.Code)
	}
	var originals, formatted []api.Document
	for i, source := range []string{"original", "linkedin", "jobstreet", "other"} {
		originals = append(originals, document(create(path, source, i)))
	}
	for i := range 3 {
		formatted = append(formatted, document(create(path, "formatted", 4+i)))
	}
	isLimit(create(path, "original", 7))
	isLimit(create(path, "formatted", 8))
	isLimit(create(path, "original", 7, formatted[0].Id))
	foreign := document(create("/v1/candidates/"+other.Id+"/documents", "original", 9))
	isLimit(create(path, "original", 7, foreign.Id))
	bad := a.upload(path, recruiter, "original", "unreadable.exe", []byte("x"), originals[0].Id)
	require.Equal(t, http.StatusUnsupportedMediaType, bad.Status)
	require.Empty(t, a.binList(admin), "rejected replacements leave every existing file active")
	first := document(create(path, "original", 7, originals[0].Id))
	document(create(path, "formatted", 8, formatted[0].Id))
	isLimit(create(path, "original", 10, originals[0].Id))

	files := string(a.request(http.MethodGet, path, recruiter, nil).Body)
	assert.Contains(t, files, first.Id)
	assert.NotContains(t, files, originals[0].Id)
	assert.NotContains(t, files, formatted[0].Id)
	assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/documents/"+originals[0].Id+"/file", recruiter, nil).Status)
	entries := a.binList(admin)
	require.Len(t, entries, 2)
	entriesByRecord := map[string]api.BinEntry{}
	for _, e := range entries {
		assert.Equal(t, api.BinKindDocument, e.Kind)
		assert.WithinDuration(t, e.BinnedAt.Add(30*24*time.Hour), e.PurgeAfter, time.Second)
		entriesByRecord[e.RecordId] = e
	}
	restore := "/v1/bin/" + entriesByRecord[originals[0].Id].Id + "/restore"
	assert.Equal(t, http.StatusConflict, a.request(http.MethodPost, restore, admin, nil).Status)
	require.Equal(t, http.StatusCreated, a.bin(admin, map[string]any{"kind": "document", "id": first.Id}).Status)
	require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, restore, admin, nil).Status)
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/documents/"+originals[0].Id+"/file", recruiter, nil).Status)

	filesStore, err := blob.New(filesDir, nil, "")
	require.NoError(t, err)
	for _, d := range []api.Document{first, formatted[0]} {
		f, err := filesStore.Open(tenant.ID + "/" + d.Id)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}
	purgeNow(t, a, tenant.ID)
	assert.Empty(t, a.binList(admin))
	for _, d := range []api.Document{first, formatted[0]} {
		var found int
		require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT count(*) FROM document WHERE id=$1", d.Id).Scan(&found))
		assert.Zero(t, found)
		_, err := filesStore.Open(tenant.ID + "/" + d.Id)
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
	assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/v1/documents/"+originals[0].Id+"/file", recruiter, nil).Status)
}
