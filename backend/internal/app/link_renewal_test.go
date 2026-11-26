package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

const linkExpired = "LinkExpiredError: link expired (HTTP 403): the file's link no longer works; read it again"

// From the owner's post-migration report (P1): a CV waiting in the queue
// past its link's life, or held after a restore from another server, failed
// with "read it again" though its file was there. Its job is queued again
// with a new link, and the CV is read; a link that still works here is not
// renewed (whatever refused it would refuse a new one), so that CV fails.
func TestACVWhoseLinkNoLongerWorksIsReadWithANewLink(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	conn, err := pgx.Connect(ctx, testutil.Current.AIURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()
	upload := func(name string) api.Document {
		c := a.createCandidate(token, map[string]any{"firstName": name})
		res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.pdf", minimalPDF)
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var doc api.Document
		res.JSON(&doc)
		return doc
	}

	// Signed by the server the backup came from: this one refuses it.
	doc := upload("Nurul")
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET payload = jsonb_set(payload, '{downloadUrl}',
		to_jsonb(regexp_replace(payload->>'downloadUrl', 'sig=[0-9a-f]+', 'sig=' || repeat('0', 64))))
		WHERE tenant_id = $1 AND payload->>'documentId' = $2`, tenant.ID, doc.Id)
	require.NoError(t, err)
	old := claimAIJobFor(t, conn, doc.Id)
	failAIJob(t, conn, old, true, linkExpired)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	assert.NotEqual(t, api.DocumentStatusFailed, a.getDocument(token, doc.Id).Status, "not failed: its file is there")
	again := claimAIJobFor(t, conn, doc.Id)
	assert.Equal(t, old.Kind, again.Kind, "the same work, queued again")
	var before, after struct{ DownloadUrl string }
	require.NoError(t, json.Unmarshal(old.Payload, &before))
	require.NoError(t, json.Unmarshal(again.Payload, &after))
	require.NotEqual(t, before.DownloadUrl, after.DownloadUrl)
	link, err := url.Parse(after.DownloadUrl)
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(link.Query().Get("sig"), "0000"), "signed by this server")
	res := a.request(http.MethodGet, link.RequestURI(), "", nil)
	assert.Equal(t, http.StatusOK, res.Status, "the new link downloads the CV")

	// A link that still works here is not renewed: the CV says it failed.
	other := upload("Wei")
	failAIJob(t, conn, claimAIJobFor(t, conn, other.Id), true, linkExpired)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.DocumentStatusFailed, a.getDocument(token, other.Id).Status)
	assert.Equal(t, 0, activeReadingJobs(t, other.Id))
}
