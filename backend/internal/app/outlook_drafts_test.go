package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type draftProvider struct {
	fallback     *outlookMailFixture
	mu           sync.Mutex
	drafts       map[string]map[string]any
	sequence     int
	attempts     int
	accepted     map[string]bool
	holdReceipt  bool
	lostResponse bool
	patchKeys    []string
}

func newDraftProvider() *draftProvider {
	return &draftProvider{
		fallback: &outlookMailFixture{folder: "inbox"}, drafts: map[string]map[string]any{}, accepted: map[string]bool{},
	}
}

func (f *draftProvider) updateVersion(draft map[string]any) {
	f.sequence++
	key := fmt.Sprintf("change-%d", f.sequence)
	draft["changeKey"] = key
	draft["@odata.etag"] = `W/"` + key + `"`
	draft["lastModifiedDateTime"] = time.Now().UTC()
}

func (f *draftProvider) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/me/messages") {
		f.fallback.serve(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(data any) { _ = json.NewEncoder(w).Encode(data) }
	if r.URL.Path == "/me/messages" && r.Method == http.MethodPost {
		var d map[string]any
		if json.NewDecoder(r.Body).Decode(&d) != nil {
			w.WriteHeader(400)
			return
		}
		key := fmt.Sprintf("draft-id-%d", f.sequence)
		d["id"], d["conversationId"], d["parentFolderId"], d["isDraft"] = key, key, "drafts", true
		d["createdDateTime"] = time.Now().UTC()
		d["webLink"] = "https://outlook.office.com/mail/drafts/id"
		f.updateVersion(d)
		f.drafts[key] = d
		w.WriteHeader(201)
		write(d)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/me/messages/")
	id, action, _ := strings.Cut(rest, "/")
	d, found := f.drafts[id]
	if !found {
		f.fallback.serve(w, r)
		return
	}
	if r.Method == http.MethodGet {
		if f.accepted[id] && !f.holdReceipt && d["isDraft"] == true {
			d["isDraft"] = false
			d["sentDateTime"] = time.Now().UTC()
			f.updateVersion(d)
		}
		write(d)
		return
	}
	if r.Header.Get("If-Match") != d["@odata.etag"] {
		w.WriteHeader(412)
		return
	}
	switch {
	case r.Method == http.MethodPost && action == "send":
		f.attempts++
		f.accepted[id] = true
		if f.lostResponse {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(202)
	case r.Method == http.MethodPatch:
		var patch map[string]any
		if json.NewDecoder(r.Body).Decode(&patch) != nil {
			w.WriteHeader(400)
			return
		}
		f.patchKeys = []string{}
		for key, value := range patch {
			d[key] = value
			f.patchKeys = append(f.patchKeys, key)
		}
		f.updateVersion(d)
		write(d)
	case r.Method == http.MethodDelete:
		delete(f.drafts, id)
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

func (f *draftProvider) editOutside() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, draft := range f.drafts {
		draft["subject"] = "Edited directly in Outlook"
		f.updateVersion(draft)
	}
}

func createDraftForTest(t *testing.T, a *testApp, token string) api.OutlookDraft {
	t.Helper()
	r := a.request("POST", "/v1/outlook/drafts", token, map[string]any{
		"to": []string{"nadia@example.test"}, "subject": "Interview follow-up", "body": "Thank you for speaking with us.",
	})
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var draft api.OutlookDraft
	r.JSON(&draft)
	return draft
}

func TestOutlookDraftEditsAndScheduledSendReconciliation(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	owner, coworker := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	f := newDraftProvider()
	connectTestOutlook(t, a, owner, http.HandlerFunc(f.serve))
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", owner, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	draft := createDraftForTest(t, a, owner)
	path := "/v1/outlook/drafts/" + draft.MessageId
	edit := map[string]any{"subject": "Tuesday interview", "version": draft.Version}
	r := a.request("PATCH", path, owner, edit)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&draft)
	assert.Equal(t, "Thank you for speaking with us.", draft.Body)
	f.mu.Lock()
	keys := append([]string(nil), f.patchKeys...)
	f.mu.Unlock()
	assert.Equal(t, []string{"subject"}, keys, "editing the subject leaves formatting and attachments untouched")
	r = a.request("PATCH", path, owner, edit)
	require.Equal(t, 409, r.Status, "%s", r.Body)
	assert.Contains(t, string(r.Body), "OUTLOOK_DRAFT_CHANGED")
	assert.Equal(t, 409, a.request("GET", path, coworker, nil).Status)

	queue := func(version string, expected int, at *time.Time) api.OutlookScheduledSend {
		r := a.request("PUT", path+"/send", owner, map[string]any{"draftVersion": version, "expectedVersion": expected, "sendAt": at})
		require.Equal(t, 202, r.Status, "%s", r.Body)
		var row api.OutlookScheduledSend
		r.JSON(&row)
		return row
	}
	list := func(token string) []api.OutlookScheduledSend {
		r := a.request("GET", "/v1/outlook/scheduled", token, nil)
		require.Equal(t, 200, r.Status, "%s", r.Body)
		var page api.OutlookScheduleList
		r.JSON(&page)
		return page.Items
	}
	statusOf := func(id string) api.OutlookScheduledSendStatus {
		t.Helper()
		for _, row := range list(owner) {
			if row.Id == id {
				return row.Status
			}
		}
		t.Fatalf("schedule missing: %s", id)
		return ""
	}
	at := time.Now().Add(time.Hour)
	planned := queue(draft.Version, 0, &at)
	later := at.Add(time.Hour)
	moved := queue(draft.Version, planned.Version, &later)
	require.Equal(t, planned.Id, moved.Id)
	assert.Greater(t, moved.Version, planned.Version)
	assert.True(t, later.Truncate(time.Second).Equal(moved.SendAt.Truncate(time.Second)))
	require.Empty(t, list(coworker))
	cancel := "/v1/outlook/scheduled/" + moved.Id + "/cancel"
	assert.Equal(t, 404, a.request("POST", cancel, coworker, map[string]int{"version": moved.Version}).Status)
	assert.Equal(t, 409, a.request("POST", cancel, owner, map[string]int{"version": planned.Version}).Status)
	assert.Equal(t, 200, a.request("POST", cancel, owner, map[string]int{"version": moved.Version}).Status)
	assert.Equal(t, api.OutlookScheduledSendStatusCancelled, statusOf(moved.Id))

	overdue := func() {
		_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE job SET run_after=now() WHERE tenant_id=$1 AND kind='email.outlook_scheduled' AND status='pending'", tenant.ID)
		require.NoError(t, err)
		runCoreJobsUntilIdle(t, a, tenant.ID)
	}
	changed := queue(draft.Version, 0, &at)
	f.editOutside()
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_send SET send_at=now()-interval '1 minute' WHERE id=$1", changed.Id)
	require.NoError(t, err)
	overdue()
	assert.Equal(t, api.OutlookScheduledSendStatusNeedsReview, statusOf(changed.Id))
	f.mu.Lock()
	attempted := f.attempts
	f.lostResponse, f.holdReceipt = true, true
	f.mu.Unlock()
	assert.Zero(t, attempted, "an externally edited draft is never sent automatically")
	a.request("GET", path, owner, nil).JSON(&draft)
	sent := queue(draft.Version, changed.Version, nil)
	require.Equal(t, changed.Id, sent.Id)
	assert.Equal(t, 409, a.request("POST", "/v1/outlook/scheduled/"+changed.Id+"/cancel", owner, map[string]int{"version": changed.Version}).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Equal(t, api.OutlookScheduledSendStatusConfirming, statusOf(sent.Id))
	assert.Equal(t, 409, a.request("POST", "/v1/outlook/scheduled/"+sent.Id+"/cancel", owner, map[string]int{"version": sent.Version}).Status)
	f.mu.Lock()
	f.holdReceipt = false
	f.mu.Unlock()
	overdue()
	assert.Equal(t, api.OutlookScheduledSendStatusSent, statusOf(sent.Id))
	f.mu.Lock()
	attempted = f.attempts
	f.mu.Unlock()
	assert.Equal(t, 1, attempted, "losing Graph's response is not permission to post again")
}

func TestOutlookDisconnectCancelsScheduledDrafts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter].Token
	f := newDraftProvider()
	connectTestOutlook(t, a, user, http.HandlerFunc(f.serve))
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", user, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	draft := createDraftForTest(t, a, user)
	path := "/v1/outlook/drafts/" + draft.MessageId
	r := a.request("PUT", path+"/send", user, map[string]any{"draftVersion": draft.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
	require.Equal(t, 202, r.Status, "%s", r.Body)
	assert.Equal(t, 409, a.request("DELETE", path+"?version="+draft.Version, user, nil).Status)
	require.Equal(t, 204, a.request("DELETE", "/v1/mailbox", user, nil).Status)
	var list api.OutlookScheduleList
	a.request("GET", "/v1/outlook/scheduled", user, nil).JSON(&list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, api.OutlookScheduledSendStatusCancelled, list.Items[0].Status)
	f.mu.Lock()
	sends, remaining := f.attempts, len(f.drafts)
	f.mu.Unlock()
	assert.Zero(t, sends)
	assert.Equal(t, 1, remaining, "disconnect leaves the original draft in Outlook")
}

func TestOutlookInboxRulesGrantKeepsScheduledDrafts(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter].Token
	drafts, rules := newDraftProvider(), newOutlookRulesFixture()
	connectTestOutlook(t, a, user, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me", "/token", "/me/mailFolders/inbox/messageRules":
			rules.serve(w, r)
		default:
			drafts.serve(w, r)
		}
	}))
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", user, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	draft := createDraftForTest(t, a, user)
	path := "/v1/outlook/drafts/" + draft.MessageId
	r := a.request("PUT", path+"/send", user, map[string]any{"draftVersion": draft.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
	require.Equal(t, 202, r.Status, "%s", r.Body)
	var planned api.OutlookScheduledSend
	r.JSON(&planned)

	require.Equal(t, 200, authorizeInboxRules(t, a, user, "rules-same").Status)
	require.Equal(t, 200, a.request("GET", path, user, nil).Status)
	var list api.OutlookScheduleList
	r = a.request("GET", "/v1/outlook/scheduled", user, nil)
	require.Equal(t, 200, r.Status)
	r.JSON(&list)
	require.Equal(t, []api.OutlookScheduledSend{planned}, list.Items)
}
