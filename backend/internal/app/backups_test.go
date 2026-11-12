package app_test

import (
	"archive/tar"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

const backupName = "hiringplug-20261101-020000"

// aBackupIn writes a small backup folder, as `hp backup` makes them.
func aBackupIn(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, backupName)
	require.NoError(t, os.MkdirAll(path, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "database.dump"), bytes.Repeat([]byte("d"), 2000), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "files.tar.gz"), bytes.Repeat([]byte("f"), 700), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "manifest.json"), []byte(`{"createdAt":"2026-11-01T02:00:03Z","files":4}`), 0o600))
}

// Settings › Backups: the backups with their sizes, the nightly switch, "Back
// up now", a download link that works without signing in for 10 minutes,
// and deleting one; only for the owner of the agency that runs the server.
// Not parallel: the switch is one row for the whole server.
func TestBackupsInSettings(t *testing.T) {
	dir := t.TempDir()
	aBackupIn(t, dir)
	operator := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin)
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	a := newTestApp(t, map[string]string{"HP_BACKUP_DIR": dir, "HP_BACKUP_TENANT": operator.ID})
	owner := operator.Users[api.RoleOwner].Token
	t.Cleanup(func() { a.request(http.MethodPut, "/v1/settings/backups/auto", owner, map[string]any{"auto": true}) })

	var b api.Backups
	a.request(http.MethodGet, "/v1/settings/backups", owner, nil).JSON(&b)
	require.True(t, b.Available)
	require.Len(t, b.Backups, 1)
	assert.Equal(t, backupName, b.Backups[0].Name)
	assert.Equal(t, 4, b.Backups[0].Files)
	// The .tar: the folder and three files, each a 512-byte header and its
	// content in 512-byte blocks, then two empty blocks.
	wantSize := int64(512 + (512 + 2048) + (512 + 1024) + (512 + 512) + 1024)
	assert.Equal(t, wantSize, b.Backups[0].SizeBytes)
	assert.Equal(t, wantSize, b.TotalBytes)

	t.Run("only the owner of the agency that runs the server", func(t *testing.T) {
		res := a.request(http.MethodGet, "/v1/settings/backups", operator.Users[api.RoleAdmin].Token, nil)
		assert.Equal(t, http.StatusForbidden, res.Status, "admins of the same agency: no")
		var theirs api.Backups
		a.request(http.MethodGet, "/v1/settings/backups", other.Users[api.RoleOwner].Token, nil).JSON(&theirs)
		assert.False(t, theirs.Available, "another agency's owner sees nothing")
		assert.Empty(t, theirs.Backups)
		for _, call := range []struct{ method, path string }{
			{http.MethodPut, "/v1/settings/backups/auto"}, {http.MethodPost, "/v1/settings/backups/run"},
			{http.MethodPost, "/v1/settings/backups/" + backupName + "/link"}, {http.MethodDelete, "/v1/settings/backups/" + backupName},
		} {
			res := a.request(call.method, call.path, other.Users[api.RoleOwner].Token, map[string]any{"auto": false})
			assert.Equal(t, http.StatusForbidden, res.Status, call.path)
		}

		// Without HP_BACKUP_TENANT, only the agency of a one-agency server
		// (the test database holds many).
		unset := newTestApp(t, map[string]string{"HP_BACKUP_DIR": dir})
		var none api.Backups
		unset.request(http.MethodGet, "/v1/settings/backups", owner, nil).JSON(&none)
		assert.False(t, none.Available)
	})

	t.Run("the nightly switch and Back up now", func(t *testing.T) {
		a.request(http.MethodPut, "/v1/settings/backups/auto", owner, map[string]any{"auto": false}).JSON(&b)
		assert.False(t, b.Auto)
		res := a.request(http.MethodPost, "/v1/settings/backups/run", owner, nil)
		require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
		res.JSON(&b)
		require.True(t, b.RequestedAt.IsSpecified())
		assert.False(t, b.RequestedAt.IsNull(), "asked for: the backup service makes it")
	})

	t.Run("download with a short-lived link, no sign-in needed", func(t *testing.T) {
		var link api.BackupDownloadLink
		res := a.request(http.MethodPost, "/v1/settings/backups/"+backupName+"/link", owner, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&link)
		assert.Equal(t, backupName+".tar", link.FileName)
		assert.Equal(t, wantSize, link.SizeBytes)
		assert.WithinDuration(t, time.Now().Add(10*time.Minute), link.ExpiresAt, 5*time.Second)

		dl := a.request(http.MethodGet, link.Url, "", nil)
		require.Equal(t, http.StatusOK, dl.Status, "%s", dl.Body)
		assert.Equal(t, strconv.FormatInt(wantSize, 10), dl.Header.Get("Content-Length"))
		assert.Contains(t, dl.Header.Get("Content-Disposition"), backupName+".tar")
		assert.Len(t, dl.Body, int(wantSize))
		names := []string{}
		tr := tar.NewReader(bytes.NewReader(dl.Body))
		for {
			hdr, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			names = append(names, hdr.Name)
		}
		assert.Equal(t, []string{backupName + "/", backupName + "/database.dump", backupName + "/files.tar.gz", backupName + "/manifest.json"}, names)

		tampered := a.request(http.MethodGet, "/v1/backup-downloads/"+backupName+"?exp=9999999999&sig="+hex.EncodeToString(make([]byte, 32)), "", nil)
		assert.Equal(t, http.StatusForbidden, tampered.Status, "a made-up signature")
		past := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
		m := hmac.New(sha256.New, []byte("files-secret-for-tests-0123456789abcdef"))
		_, _ = fmt.Fprintf(m, "backup-download\n%s\n%s", backupName, past)
		expired := a.request(http.MethodGet, "/v1/backup-downloads/"+backupName+"?exp="+past+"&sig="+hex.EncodeToString(m.Sum(nil)), "", nil)
		assert.Equal(t, http.StatusForbidden, expired.Status, "an expired link")
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/backup-downloads/..%2Fsecrets?exp=1&sig=x", "", nil).Status, "only backups")
		assert.Equal(t, http.StatusBadRequest, a.request(http.MethodGet, "/v1/backup-downloads/secrets?exp=1&sig=x", "", nil).Status, "only backups")
	})

	t.Run("delete one", func(t *testing.T) {
		res := a.request(http.MethodDelete, "/v1/settings/backups/"+backupName, owner, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		res.JSON(&b)
		assert.Empty(t, b.Backups)
		assert.Zero(t, b.TotalBytes)
		_, err := os.Stat(filepath.Join(dir, backupName))
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodDelete, "/v1/settings/backups/"+backupName, owner, nil).Status)
	})
}
