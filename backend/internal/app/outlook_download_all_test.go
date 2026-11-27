package app_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Two files named alike, a picture in the text, a cloud link and a file too large.
func serveDownloadAllFixture(w http.ResponseWriter, r *http.Request) {
	const root = "/me/messages/remote-message/attachments"
	file := func(id, name string, size int) map[string]any {
		return map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": id, "name": name, "size": size}
	}
	first, second := "%PDF first CV", "%PDF second CV"
	inline := file("logo", "signature.png", 5)
	inline["isInline"] = true
	items := []any{file("cv-1", "CV.pdf", len(first)), file("cv-2", "cv.PDF", len(second)), inline,
		map[string]any{"@odata.type": "#microsoft.graph.referenceAttachment", "id": "link", "name": "Work samples", "size": 0},
		file("big", "Portfolio.pdf", 21<<20)}
	switch r.URL.Path {
	case root:
		_ = json.NewEncoder(w).Encode(map[string]any{"value": items})
	case root + "/cv-1/$value":
		_, _ = io.WriteString(w, first)
	case root + "/cv-2/$value":
		_, _ = io.WriteString(w, second)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Download all: every file attachment of a message in one .zip, as Outlook
// does; only for the mailbox's owner.
func TestDownloadAllZipsAMessagesAttachments(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	recruiter, owner := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	f := &outlookMailFixture{folder: "inbox"}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/me/messages/remote-message/attachments") {
			serveDownloadAllFixture(w, r)
			return
		}
		f.serve(w, r)
	})
	for _, token := range []string{recruiter, owner} {
		connectTestOutlook(t, a, token, handler)
		require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	}
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var mailbox api.OutlookMailPage
	a.request("GET", "/v1/outlook/mail", recruiter, nil).JSON(&mailbox)
	require.Len(t, mailbox.Items, 1)
	path := "/v1/outlook/messages/" + mailbox.Items[0].Id + "/attachments/zip"

	assert.Equal(t, 404, a.request("GET", path, owner, nil).Status, "another person's mail")
	res := a.request("GET", path, recruiter, nil)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	assert.Contains(t, res.Header.Get("Content-Disposition"), "Attachments.zip")
	archive, err := zip.NewReader(bytes.NewReader(res.Body), int64(len(res.Body)))
	require.NoError(t, err)
	got := map[string]string{}
	for _, file := range archive.File {
		rc, err := file.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		got[file.Name] = string(data)
	}
	assert.Equal(t, "%PDF first CV", got["CV.pdf"])
	assert.Equal(t, "%PDF second CV", got["cv (2).PDF"], "a second file of the same name is kept apart")
	assert.Contains(t, got["Left out.txt"], "Portfolio.pdf: over 20 MB")
	assert.Len(t, got, 3, "no picture from the text, no cloud link")
}
