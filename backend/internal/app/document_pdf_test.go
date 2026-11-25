package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Review of 1.47.0: a Word CV whose PDF copy failed (its file unreadable
// after a restore, say) kept "No PDF copy" for good: asking for one did
// nothing once it had failed. A person asking ("Make the PDF again") queues
// it again, through a newly signed link; a page looking at it (no again)
// never does, so a failure is not retried on every view.
func TestAFailedPDFCopyCanBeMadeAgain(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	token := tenant.Users[api.RoleRecruiter].Token
	admin := testutil.Current.Admin
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM job WHERE queue = 'ai' AND tenant_id = $1`, tenant.ID)
	})
	c := a.createCandidate(token, map[string]any{"firstName": "Lim"})
	res := a.upload("/v1/candidates/"+c.Id+"/documents", token, "original", "cv.docx", minimalDOCX(t))
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var word api.Document
	res.JSON(&word)
	_, err := admin.Exec(ctx, `UPDATE job SET status = 'dead', finished_at = now(), last_error = 'DownloadGoneError: HTTP 403'
		WHERE kind = 'document.render_pdf' AND payload->>'documentId' = $1`, word.Id)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Equal(t, api.PdfStatusFailed, a.getDocument(token, word.Id).PdfStatus.MustGet())
	renders := func() []string {
		var urls []string
		rows, err := admin.Query(ctx, `SELECT payload->>'downloadUrl' FROM job WHERE kind = 'document.render_pdf' AND status = 'pending'
			AND payload->>'documentId' = $1`, word.Id)
		require.NoError(t, err)
		for rows.Next() {
			var u string
			require.NoError(t, rows.Scan(&u))
			urls = append(urls, u)
		}
		return urls
	}

	res = a.request(http.MethodPost, "/v1/documents/"+word.Id+"/pdf", token, nil)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	assert.Equal(t, api.PdfStatusFailed, a.getDocument(token, word.Id).PdfStatus.MustGet(), "looked at: left as it is")
	assert.Empty(t, renders())

	res = a.request(http.MethodPost, "/v1/documents/"+word.Id+"/pdf", tenant.Users[api.RoleViewer].Token, map[string]any{"again": true})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	var again api.Document
	require.NoError(t, json.Unmarshal(res.Body, &again))
	assert.Equal(t, api.PdfStatusQueued, again.PdfStatus.MustGet())
	urls := renders()
	require.Len(t, urls, 1)
	u, err := url.Parse(urls[0])
	require.NoError(t, err)
	exp, err := strconv.ParseInt(u.Query().Get("exp"), 10, 64)
	require.NoError(t, err)
	assert.Greater(t, time.Unix(exp, 0), time.Now().Add(24*time.Hour), "a newly signed link")
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/documents/"+word.Id+"/pdf", token, map[string]any{"again": true}).Status)
	assert.Len(t, renders(), 1, "being made: never queued twice")
}
