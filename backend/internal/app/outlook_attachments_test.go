package app_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

const outlookFixtureCV = "%PDF-1.7\nSynthetic candidate CV\n%%EOF"

func serveOutlookAttachmentFixture(w http.ResponseWriter, r *http.Request) {
	const root = "/me/messages/remote-message/attachments"
	file := map[string]any{
		"@odata.type": "#microsoft.graph.fileAttachment",
		"id":          "file/01+=",
		"name":        "uploads\\Nadia – CV.pdf",
		"size":        len(outlookFixtureCV),
	}
	large := map[string]any{
		"@odata.type": "#microsoft.graph.fileAttachment", "id": "large", "name": "Portfolio.pdf", "size": 21 << 20,
	}
	reference := map[string]any{
		"@odata.type": "#microsoft.graph.referenceAttachment", "id": "cloud-link", "name": "Work samples", "size": 0,
	}
	inline := map[string]any{
		"@odata.type": "#microsoft.graph.fileAttachment", "id": "inline-logo", "name": "signature.png", "isInline": true, "size": 5,
	}
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch r.URL.Path {
	case root:
		write(map[string]any{"value": []any{file, large, reference, inline}})
	case root + "/file/01+=":
		write(file)
	case root + "/file/01+=/$value":
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, outlookFixtureCV)
	case root + "/large":
		write(large)
	case root + "/cloud-link":
		write(reference)
	case root + "/inline-logo":
		write(inline)
	case root + "/oversized-stream":
		file["id"] = "oversized-stream"
		write(file)
	case root + "/oversized-stream/$value":
		w.Header().Set("Content-Type", "application/octet-stream")
		chunk := strings.Repeat("a", 64<<10)
		for range 321 {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestOutlookAttachmentsArePrivateAndDownloadOriginalFiles(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	recruiter, owner := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	setupOutlookMail(t, a, recruiter)
	setupOutlookMail(t, a, owner)
	for _, token := range []string{recruiter, owner} {
		require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	}
	runCoreJobsUntilIdle(t, a, tenant.ID)
	messages := a.request("GET", "/v1/outlook/mail", recruiter, nil)
	require.Equal(t, 200, messages.Status)
	var mailbox api.OutlookMailPage
	messages.JSON(&mailbox)
	require.Len(t, mailbox.Items, 1)

	root := "/v1/outlook/messages/" + mailbox.Items[0].Id + "/attachments"
	assert.Equal(t, 401, a.request("GET", root, "", nil).Status)
	assert.Equal(t, 404, a.request("GET", root, owner, nil).Status)
	list := a.request("GET", root, recruiter, nil)
	require.Equal(t, 200, list.Status, "%s", list.Body)
	assert.Equal(t, "no-store", list.Header.Get("Cache-Control"))
	var attachments api.OutlookAttachmentList
	list.JSON(&attachments)
	require.Len(t, attachments.Items, 3, "inline signature images are not shown")
	cv := attachments.Items[0]
	require.Equal(t, "Nadia – CV.pdf", cv.FileName)
	require.True(t, cv.Downloadable)
	assert.False(t, attachments.Items[1].Downloadable)
	assert.False(t, attachments.Items[2].Downloadable)
	assert.NotContains(t, string(list.Body), outlookFixtureCV)

	filePath := func(id string) string { return root + "/" + id + "/file" }
	assert.Equal(t, 404, a.request("GET", filePath(cv.Id), owner, nil).Status)
	file := a.request("GET", filePath(cv.Id), recruiter, nil)
	require.Equal(t, 200, file.Status, "%s", file.Body)
	assert.Equal(t, outlookFixtureCV, string(file.Body))
	assert.Equal(t, "application/octet-stream", file.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", file.Header.Get("Cache-Control"))
	disposition, params, err := mime.ParseMediaType(file.Header.Get("Content-Disposition"))
	require.NoError(t, err)
	assert.Equal(t, "attachment", disposition)
	assert.Equal(t, cv.FileName, params["filename"])

	encoded := func(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
	for _, id := range []string{"inline-logo", "cloud-link", "../other-message", "missing"} {
		assert.Equal(t, 404, a.request("GET", filePath(encoded(id)), recruiter, nil).Status, id)
	}
	for _, id := range []string{"large", "oversized-stream"} {
		larger := a.request("GET", filePath(encoded(id)), recruiter, nil)
		require.Equal(t, 413, larger.Status, id)
		assert.Equal(t, api.ErrorCodeOUTLOOKATTACHMENTTOOLARGE, larger.Error().Error.Code)
	}
}
