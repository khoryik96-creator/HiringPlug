package app_test

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type forwardMailFixture struct {
	*draftFilesProvider
	forwards int
}

func newForwardMailFixture() *forwardMailFixture {
	return &forwardMailFixture{draftFilesProvider: newDraftFilesProvider()}
}

func (f *forwardMailFixture) serve(w http.ResponseWriter, r *http.Request) {
	const original = "/me/messages/remote-message/"
	const draft = "/me/messages/forward-draft/"
	if strings.HasPrefix(r.URL.Path, draft+"attachments") {
		f.draftFilesProvider.serve(w, r)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != original+"createForward" {
		f.draftProvider.serve(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forwards++
	var in map[string]json.RawMessage
	var message map[string]json.RawMessage
	var recipients []map[string]map[string]string
	var note string
	if json.NewDecoder(r.Body).Decode(&in) != nil ||
		json.Unmarshal(in["message"], &message) != nil || len(message) != 1 ||
		json.Unmarshal(message["toRecipients"], &recipients) != nil || len(recipients) == 0 ||
		(in["comment"] != nil && json.Unmarshal(in["comment"], &note) != nil) ||
		!strings.Contains(r.Header.Get("Prefer"), `IdType="ImmutableId"`) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	for _, recipient := range recipients {
		if recipient["emailAddress"]["address"] == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	payload := map[string]any{
		"id": "forward-draft", "parentFolderId": "drafts", "conversationId": "thread-one",
		"isDraft": true, "hasAttachments": true, "createdDateTime": time.Now().UTC(),
		"subject": "FW: Availability for the interview", "toRecipients": recipients,
		"body":    map[string]string{"contentType": "html", "content": "<p>" + html.EscapeString(note) + "</p><p>I can make Tuesday.</p>"},
		"webLink": "https://outlook.office.com/mail/drafts/id",
	}
	f.updateVersion(payload)
	f.drafts["forward-draft"] = payload
	f.files["forward-draft"] = []map[string]any{{
		"id": "copied-file", "name": "Nadia – CV.pdf", "size": len(outlookFixtureCV), "@odata.type": "#microsoft.graph.fileAttachment",
	}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(payload)
}

func TestOutlookForwardCreatesAPrivateDraftWithTheOriginalFiles(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	user, other := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	f := newForwardMailFixture()
	for _, token := range []string{user, other} {
		connectTestOutlook(t, a, token, http.HandlerFunc(f.serve))
		require.Equal(t, http.StatusNoContent, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	}
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var inbox api.OutlookMailPage
	a.request("GET", "/v1/outlook/mail", user, nil).JSON(&inbox)
	require.Len(t, inbox.Items, 1)
	source := "/v1/outlook/messages/" + inbox.Items[0].Id + "/forward"
	recipients := []string{"james@example.test", "lina@example.test"}
	body := map[string]any{"to": recipients, "note": "Please review Nadia's availability."}

	require.Equal(t, http.StatusNotFound, a.request("POST", source, other, body).Status)
	require.Equal(t, http.StatusBadRequest, a.request("POST", source, user, map[string]any{"to": []string{}}).Status)
	r := a.request("POST", source, user, body)
	require.Equal(t, http.StatusCreated, r.Status, "%s", r.Body)
	var draft api.OutlookDraft
	r.JSON(&draft)
	assert.NotEqual(t, inbox.Items[0].Id, draft.MessageId)
	assert.Equal(t, recipients, draft.To)
	assert.True(t, draft.HasAttachments)
	assert.Contains(t, draft.Subject, "FW: Availability for the interview")
	assert.Contains(t, draft.Body, "Please review Nadia's availability.")
	assert.Contains(t, draft.Body, "I can make Tuesday.")

	draftPath := "/v1/outlook/drafts/" + draft.MessageId
	r = a.request("GET", draftPath, user, nil)
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var reopened api.OutlookDraft
	r.JSON(&reopened)
	assert.Equal(t, draft.Version, reopened.Version)
	assert.Equal(t, http.StatusBadRequest, a.request("POST", "/v1/outlook/messages/"+draft.MessageId+"/forward", user, body).Status)

	r = a.request("GET", "/v1/outlook/messages/"+draft.MessageId+"/attachments", user, nil)
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var files api.OutlookAttachmentList
	r.JSON(&files)
	require.NotEmpty(t, files.Items)
	assert.Equal(t, "Nadia – CV.pdf", files.Items[0].FileName)
	r = a.request("PUT", draftPath+"/send", user, map[string]any{
		"draftVersion": reopened.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour),
	})
	require.Equal(t, http.StatusAccepted, r.Status, "%s", r.Body)
	r = addDraftFileForTest(t, a, user, draft.MessageId, reopened.Version)
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var attached api.OutlookDraft
	r.JSON(&attached)
	assert.True(t, attached.ScheduleNeedsReview)
	remove := draftPath + "/attachments/" + files.Items[0].Id + "?version=" + url.QueryEscape(attached.Version)
	r = a.request("DELETE", remove, user, nil)
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var updated api.OutlookDraft
	r.JSON(&updated)
	assert.Equal(t, draft.Body, updated.Body)
	assert.True(t, updated.ScheduleNeedsReview)
	a.request("GET", "/v1/outlook/messages/"+draft.MessageId+"/attachments", user, nil).JSON(&files)
	require.Len(t, files.Items, 1)
	assert.Equal(t, "Resume.pdf", files.Items[0].FileName)
	a.request("GET", "/v1/outlook/messages/"+inbox.Items[0].Id+"/attachments", user, nil).JSON(&files)
	require.NotEmpty(t, files.Items)
	assert.Equal(t, "Nadia – CV.pdf", files.Items[0].FileName, "editing the forward leaves the original message alone")
	f.mu.Lock()
	forwards, sends := f.forwards, f.attempts
	f.mu.Unlock()
	assert.Equal(t, 1, forwards)
	assert.Zero(t, sends, "creating a forward never sends it")
}
