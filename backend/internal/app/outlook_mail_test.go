package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type outlookMailFixture struct {
	mu       sync.Mutex
	read     bool
	folder   string
	reply    string
	draft    string
	failSend bool
	flag     string // the message's flag status ("" when never flagged)
}

func (f *outlookMailFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	folder := func(id string) map[string]any {
		return map[string]any{"id": id, "displayName": map[string]string{"inbox": "Inbox", "archive": "Archive", "sentitems": "Sent Items", "drafts": "Drafts", "deleteditems": "Deleted Items", "junkemail": "Junk Email"}[id], "childFolderCount": 0}
	}
	switch {
	case r.URL.Path == "/token":
		write(map[string]any{"access_token": "fake-outlook-token", "refresh_token": "fake-refresh", "expires_in": 3600, "token_type": "Bearer"})
	case r.URL.Path == "/profile":
		write(map[string]string{"mail": "recruiter@example.test"})
	case strings.HasSuffix(r.URL.Path, "/mailFolders"):
		write(map[string]any{"value": []any{folder("inbox"), folder("sentitems"), folder("drafts"), folder("deleteditems"), folder("junkemail"), folder("archive")}})
	case strings.HasSuffix(r.URL.Path, "/messages/delta"):
		parts := strings.Split(r.URL.Path, "/")
		var id string
		if parts[2] == "mailFolders" {
			id = parts[3]
		} else {
			id = strings.TrimSuffix(strings.TrimPrefix(parts[2], "mailfolders('"), "')")
		}
		items := []any{}
		if id == f.folder {
			now := time.Now().UTC().Truncate(time.Millisecond)
			items = append(items, map[string]any{
				"id": "remote-message", "parentFolderId": id, "conversationId": "thread-one",
				"subject":          "Availability for the interview",
				"from":             map[string]any{"emailAddress": map[string]string{"name": "Nadia Rahman", "address": "nadia@example.test"}},
				"flag":             map[string]string{"flagStatus": f.flag},
				"toRecipients":     []any{outlookRecipientJSON("recruiter@example.test")},
				"receivedDateTime": now, "lastModifiedDateTime": now, "isRead": f.read,
				"hasAttachments": true, "webLink": "https://outlook.office.com/mail/item",
			})
		} else if id == "inbox" && r.URL.Query().Has("$deltatoken") {
			items = append(items, map[string]any{"id": "remote-message", "@removed": map[string]string{"reason": "deleted"}})
		}
		// Microsoft emits this OData folder syntax in continuation URLs.
		write(map[string]any{"value": items, "@odata.deltaLink": "http://" + r.Host + "/me/mailfolders('" + id + "')/messages/delta?$deltatoken=one"})
	case strings.HasPrefix(r.URL.Path, "/me/mailFolders/"):
		write(folder(strings.TrimPrefix(r.URL.Path, "/me/mailFolders/")))
	case strings.HasPrefix(r.URL.Path, "/me/messages/remote-message/attachments"):
		serveOutlookAttachmentFixture(w, r)
	case r.URL.Path == "/me/messages/remote-message" && r.Method == "GET":
		write(map[string]any{"body": map[string]string{"contentType": "html", "content": "<p>I can make Tuesday.</p><script>unsafe()</script>"}})
	case r.URL.Path == "/me/messages/remote-message" && r.Method == "PATCH":
		var body struct{ IsRead bool }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.read = body.IsRead
		write(map[string]any{"lastModifiedDateTime": time.Now().UTC()})
	case r.URL.Path == "/me/messages/remote-message/move":
		var body struct{ DestinationID string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.folder = body.DestinationID
		write(map[string]any{"id": "remote-message", "lastModifiedDateTime": time.Now().UTC()})
	case r.URL.Path == "/me/messages/remote-message/replyAll":
		var body struct{ Comment string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.reply = body.Comment
		if f.failSend {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(202)
	case r.URL.Path == "/me/messages/remote-message/createReply":
		var body struct{ Comment string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.draft = body.Comment
		w.WriteHeader(201)
		write(map[string]string{"id": "reply-draft"})
	default:
		w.WriteHeader(404)
	}
}

func outlookRecipientJSON(address string) map[string]any {
	return map[string]any{"emailAddress": map[string]string{"address": address}}
}

func setupOutlookMail(t *testing.T, a *testApp, token string) *outlookMailFixture {
	t.Helper()
	f := &outlookMailFixture{folder: "inbox"}
	connectTestOutlook(t, a, token, http.HandlerFunc(f.serve))
	return f
}

func connectTestOutlook(t *testing.T, a *testApp, token string, handler http.Handler) {
	t.Helper()
	configureOutlookFixture(t, a, handler)
	r := a.request("POST", "/v1/mailbox/authorize", token, map[string]any{"provider": "microsoft"})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var start api.MailboxAuthorization
	r.JSON(&start)
	target, err := url.Parse(start.Url)
	require.NoError(t, err)
	require.NotContains(t, target.Query().Get("scope"), "MailboxSettings.ReadWrite")
	r = finishOutlookFixture(t, a, token, target, "synthetic-code")
	require.Equal(t, 200, r.Status, "%s", r.Body)
}

func configureOutlookFixture(t *testing.T, a *testApp, handler http.Handler) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a.modules.Emails.ConnectMailboxesWith(emails.ConnectionSettings{
		KeysSecret: []byte("synthetic-mailbox-encryption-key-32-chars"),
		PublicURL:  "http://app.test", ServerMode: "test", HTTPClient: server.Client(),
		Microsoft: emails.OAuthApplication{ClientID: "test-client", ClientSecret: "test-client-secret"},
		Providers: map[string]emails.OAuthProvider{"microsoft": {
			AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile",
			SendURL: server.URL + "/me/sendMail", Scope: "Mail.ReadWrite Mail.Send",
		}},
	})
}

func finishOutlookFixture(t *testing.T, a *testApp, token string, target *url.URL, code string) *response {
	t.Helper()
	callback := "/v1/mailbox/callback?" + url.Values{"state": {target.Query().Get("state")}, "code": {code}}.Encode()
	r := a.request("GET", callback, "", nil)
	require.Equal(t, 303, r.Status)
	return a.request("POST", "/v1/mailbox/complete", token, map[string]any{"ticket": mailboxTicket(t, r.Header.Get("Location"))})
}

func TestOutlookMailSyncActionsAndProfilePrivacy(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	token, coworker := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	f := setupOutlookMail(t, a, token)
	var list api.OutlookMailPage
	listMail := func(path, bearer string) api.OutlookMailPage {
		r := a.request("GET", path, bearer, nil)
		require.Equal(t, 200, r.Status, "%s", r.Body)
		r.JSON(&list)
		return list
	}
	root := "/v1/outlook/mail"
	assert.False(t, listMail(root, token).Enabled, "mailbox sign-in alone does not index messages")
	require.Equal(t, 204, a.request("POST", root, token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	mail := listMail(root, token)
	require.True(t, mail.Enabled)
	require.Len(t, mail.Items, 1)
	item := mail.Items[0]
	path := "/v1/outlook/messages/" + item.Id
	require.Equal(t, "Availability for the interview", item.Subject)
	assert.NotContains(t, string(a.request("GET", root, token, nil).Body), "I can make Tuesday")
	assert.Equal(t, 409, a.request("GET", path, coworker, nil).Status)
	var body api.OutlookMessageBody
	r := a.request("GET", path, token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&body)
	assert.Contains(t, body.Text, "I can make Tuesday.")
	assert.NotContains(t, body.Text, "unsafe()")
	candidate := a.createCandidate(token, map[string]any{"firstName": "Nadia", "email": "nadia@example.test"})
	other := a.createCandidate(token, map[string]any{"firstName": "Another candidate", "email": "other@example.test"})
	require.Equal(t, 400, a.request("PUT", path+"/link", token, map[string]any{"candidateId": other.Id}).Status)
	require.Equal(t, 204, a.request("PUT", path+"/link", token, map[string]any{"candidateId": candidate.Id}).Status)
	profile := root + "?candidateId=" + candidate.Id
	require.Len(t, listMail(profile, token).Items, 1)
	require.Empty(t, listMail(profile, coworker).Items, "the agency owner cannot see the recruiter's linked mail")

	require.Equal(t, 204, a.request("PATCH", path, token, map[string]bool{"isRead": true}).Status)
	require.True(t, listMail(root, token).Items[0].IsRead)
	var archive string
	for _, folder := range mail.Folders {
		if folder.Kind == "archive" {
			archive = folder.Id
		}
	}
	require.NotEmpty(t, archive)
	require.Equal(t, 204, a.request("PATCH", path, token, map[string]string{"folderId": archive}).Status)
	require.Empty(t, listMail(root, token).Items)
	require.Len(t, listMail(root+"?folder="+archive, token).Items, 1)

	reply := map[string]any{"text": "Tuesday works for us.", "mode": "replyAll", "saveDraft": false}
	r = a.request("POST", path+"/reply", token, reply)
	require.Equal(t, 202, r.Status, "%s", r.Body)
	reply["mode"], reply["saveDraft"] = "reply", true
	require.Equal(t, 202, a.request("POST", path+"/reply", token, reply).Status)
	f.mu.Lock()
	actualRead, actualFolder, actualReply, actualDraft := f.read, f.folder, f.reply, f.draft
	f.failSend = true
	f.read = false
	f.mu.Unlock()
	assert.True(t, actualRead)
	assert.Equal(t, "archive", actualFolder)
	assert.Equal(t, "Tuesday works for us.", actualReply)
	assert.Equal(t, actualReply, actualDraft)
	reply["mode"], reply["saveDraft"] = "replyAll", false
	r = a.request("POST", path+"/reply", token, reply)
	require.Equal(t, 503, r.Status, "%s", r.Body)
	assert.Contains(t, string(r.Body), "Check Outlook before trying again")

	// The old folder can report a removal after Outlook has moved the
	// immutable message. Refresh through OData continuations and keep its link.
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_sync SET requested_at=now()-interval '1 minute' WHERE tenant_id=$1", tenant.ID)
	require.NoError(t, err)
	require.Equal(t, 204, a.request("POST", root, token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID) // six folders: the sync's second pass reaches the rest
	refreshed := listMail(profile, token)
	require.Len(t, refreshed.Items, 1)
	require.Equal(t, item.Id, refreshed.Items[0].Id)
	assert.False(t, refreshed.Items[0].IsRead, "read status changes made in Outlook sync into HyPlug")
	require.Equal(t, 204, a.request("DELETE", "/v1/mailbox", token, nil).Status)
	require.False(t, listMail(root, token).Connected)
	var count int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT count(*) FROM email_outlook_message WHERE tenant_id=$1", tenant.ID).Scan(&count))
	assert.Zero(t, count, "disconnect removes the private cache")
}
