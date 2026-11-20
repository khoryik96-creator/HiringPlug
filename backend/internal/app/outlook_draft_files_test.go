package app_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type draftFilesProvider struct {
	*draftProvider
	files                  map[string][]map[string]any
	lostAttachmentResponse bool
}

func newDraftFilesProvider() *draftFilesProvider {
	return &draftFilesProvider{draftProvider: newDraftProvider(), files: map[string][]map[string]any{}}
}

func (f *draftFilesProvider) serve(w http.ResponseWriter, r *http.Request) {
	id, route, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/me/messages/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/me/messages/") || (route != "attachments" && !strings.HasPrefix(route, "attachments/")) {
		f.draftProvider.serve(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.Contains(r.Header.Get("Prefer"), `IdType="ImmutableId"`) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	draft := f.drafts[id]
	if draft == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	files := f.files[id]
	w.Header().Set("Content-Type", "application/json")
	switch {
	case route == "attachments" && r.Method == http.MethodPost:
		var upload struct {
			Type        string `json:"@odata.type"`
			Name        string
			ContentType string
			Bytes       []byte `json:"contentBytes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&upload); err != nil || upload.Type != "#microsoft.graph.fileAttachment" ||
			upload.Name != "Resume.pdf" || upload.ContentType != "application/pdf" || string(upload.Bytes) != "%PDF-test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		file := map[string]any{"id": "file-1", "name": upload.Name, "size": len(upload.Bytes), "@odata.type": upload.Type}
		f.files[id] = append(files, file)
		draft["hasAttachments"] = true
		f.updateVersion(draft)
		if f.lostAttachmentResponse {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(file)
	case route == "attachments" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{"value": append([]map[string]any{}, files...)})
	case strings.HasPrefix(route, "attachments/"):
		fileID := strings.TrimPrefix(route, "attachments/")
		index := slices.IndexFunc(files, func(file map[string]any) bool { return file["id"] == fileID })
		if index < 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(files[index])
		case http.MethodDelete:
			f.files[id] = slices.Delete(files, index, index+1)
			draft["hasAttachments"] = len(f.files[id]) > 0
			f.updateVersion(draft)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func addDraftFileForTest(t *testing.T, a *testApp, token, id, version string) *response {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "Resume.pdf")
	require.NoError(t, err)
	_, err = part.Write([]byte("%PDF-test"))
	require.NoError(t, err)
	require.NoError(t, form.Close())
	return a.request("POST", "/v1/outlook/drafts/"+id+"/attachments?version="+url.QueryEscape(version), token, &body,
		"Content-Type", form.FormDataContentType())
}

func TestOutlookDraftAttachmentsRequireScheduleReview(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	user, other := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	f := newDraftFilesProvider()
	connectTestOutlook(t, a, user, http.HandlerFunc(f.serve))
	require.Equal(t, http.StatusNoContent, a.request("POST", "/v1/outlook/mail", user, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	draft := createDraftForTest(t, a, user)
	path := "/v1/outlook/drafts/" + draft.MessageId
	filesPath := path + "/attachments"
	queue := a.request("PUT", path+"/send", user, map[string]any{
		"draftVersion": draft.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour),
	})
	require.Equal(t, http.StatusAccepted, queue.Status, "%s", queue.Body)
	var scheduled api.OutlookScheduledSend
	queue.JSON(&scheduled)

	attach := func(token, version string) *response {
		return addDraftFileForTest(t, a, token, draft.MessageId, version)
	}
	require.Equal(t, http.StatusConflict, attach(other, draft.Version).Status)
	res := attach(user, draft.Version)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var attached api.OutlookDraft
	res.JSON(&attached)
	require.True(t, attached.HasAttachments)
	require.NotEqual(t, draft.Version, attached.Version)
	require.True(t, attached.ScheduleNeedsReview)
	updatedSchedule, err := attached.Schedule.Get()
	require.NoError(t, err)
	assert.Equal(t, api.OutlookScheduledSendStatusNeedsReview, updatedSchedule.Status)
	assert.Equal(t, draft.Body, attached.Body)
	require.Equal(t, http.StatusConflict, attach(user, draft.Version).Status)

	_, err = testutil.Current.Admin.Exec(t.Context(), "UPDATE job SET run_after=now() WHERE tenant_id=$1 AND kind='email.outlook_scheduled'", tenant.ID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_send SET send_at=now() WHERE id=$1", scheduled.Id)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	attempts := f.attempts
	f.mu.Unlock()
	assert.Zero(t, attempts)

	var files api.OutlookAttachmentList
	list := a.request("GET", "/v1/outlook/messages/"+draft.MessageId+"/attachments", user, nil)
	require.Equal(t, http.StatusOK, list.Status, "%s", list.Body)
	list.JSON(&files)
	require.Len(t, files.Items, 1)
	assert.Equal(t, "Resume.pdf", files.Items[0].FileName)

	remove := filesPath + "/" + files.Items[0].Id + "?version="
	require.Equal(t, http.StatusConflict, a.request("DELETE", remove+url.QueryEscape(draft.Version), user, nil).Status)
	res = a.request("DELETE", remove+url.QueryEscape(attached.Version), user, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var removed api.OutlookDraft
	res.JSON(&removed)
	assert.False(t, removed.HasAttachments)
	assert.True(t, removed.ScheduleNeedsReview)
	assert.Equal(t, draft.Body, removed.Body)

	res = a.request("PUT", path+"/send", user, map[string]any{
		"draftVersion": removed.Version, "expectedVersion": updatedSchedule.Version, "sendAt": time.Now().Add(time.Hour),
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	f.mu.Lock()
	f.lostAttachmentResponse = true
	f.mu.Unlock()
	res = attach(user, removed.Version)
	require.Contains(t, string(res.Body), "OUTLOOK_UNAVAILABLE")
	res = a.request("GET", path, user, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var reloaded api.OutlookDraft
	res.JSON(&reloaded)
	assert.True(t, reloaded.HasAttachments)
	assert.True(t, reloaded.ScheduleNeedsReview, "a lost response still holds the send")
	updatedSchedule, err = reloaded.Schedule.Get()
	require.NoError(t, err)
	assert.Equal(t, api.OutlookScheduledSendStatusNeedsReview, updatedSchedule.Status)
}
