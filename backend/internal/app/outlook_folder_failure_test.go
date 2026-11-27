package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// A folder Outlook will not hand over (a special or shared folder) never
// blocks the others: the Inbox keeps syncing on every background pass, the
// mailbox shows no error, and that folder is tried again later.
func TestOneOutlookFolderFailingNeverBlocksTheInbox(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := &outlookMailFixture{folder: "inbox"}
	broken := 0
	connectTestOutlook(t, a, token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/mailFolders"):
			w.Header().Set("Content-Type", "application/json")
			folders := []any{}
			for _, id := range []string{"inbox", "sentitems", "drafts", "deleteditems", "junkemail", "archive", "conversation-history"} {
				folders = append(folders, map[string]any{"id": id, "displayName": id, "childFolderCount": 0})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": folders})
		case strings.Contains(r.URL.Path, "conversation-history") && strings.HasSuffix(r.URL.Path, "/messages/delta"):
			f.mu.Lock()
			broken++
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"ErrorInvalidRequest","message":"This folder cannot be synced."}}`))
		default:
			f.serve(w, r)
		}
	}))
	root := "/v1/outlook/mail"
	require.Equal(t, 204, a.request("POST", root, token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID) // seven folders: four a pass
	var mail api.OutlookMailPage
	a.request("GET", root, token, nil).JSON(&mail)
	require.Len(t, mail.Items, 1, "the Inbox synced")
	_, warned := mail.Error.Get()
	assert.Error(t, warned, "no warning while the other folders sync")
	assert.False(t, mail.Syncing, "the folder that failed waits; it is not shown as still syncing")
	tries := broken
	require.Positive(t, tries)

	// Later background passes (no refresh): the Inbox's new state comes in,
	// the failing folder is not asked again until its wait is over.
	f.mu.Lock()
	f.read = true
	f.mu.Unlock()
	_, err := testutil.Current.Admin.Exec(t.Context(),
		"UPDATE email_outlook_folder SET checked_at=now()-interval '5 minutes', cursor_url=NULL WHERE tenant_id=$1 AND remote_id='inbox'", tenant.ID)
	require.NoError(t, err)
	runPostponedCalendarJobs(t, a, tenant.ID)
	a.request("GET", root, token, nil).JSON(&mail)
	require.Len(t, mail.Items, 1)
	assert.True(t, mail.Items[0].IsRead, "the background pass reached the Inbox")
	_, warned = mail.Error.Get()
	assert.Error(t, warned, "still no warning")
	assert.Equal(t, tries, broken, "the failing folder waits its turn")

	// Its wait over, and the only folder due in that pass: still no warning.
	_, err = testutil.Current.Admin.Exec(t.Context(), `UPDATE email_outlook_folder SET checked_at = CASE WHEN remote_id='conversation-history'
		THEN now()-interval '20 minutes' ELSE now() END WHERE tenant_id=$1`, tenant.ID)
	require.NoError(t, err)
	runPostponedCalendarJobs(t, a, tenant.ID)
	a.request("GET", root, token, nil).JSON(&mail)
	_, warned = mail.Error.Get()
	assert.Error(t, warned, "no warning when only that folder was due")
	assert.Greater(t, broken, tries, "it was tried again")
}

// The Inbox itself failing is said, with Microsoft's reason; a link Outlook
// rejects is dropped, so the folder starts afresh.
func TestTheInboxFailingIsSaidWithMicrosoftsReason(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := &outlookMailFixture{folder: "inbox"}
	failing := false
	connectTestOutlook(t, a, token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fail := failing
		f.mu.Unlock()
		if fail && strings.Contains(r.URL.Path, "inbox") && strings.HasSuffix(r.URL.Path, "/messages/delta") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":"SyncStateInvalid","message":"The sync state is invalid."}}`))
			return
		}
		f.serve(w, r)
	}))
	root := "/v1/outlook/mail"
	require.Equal(t, 204, a.request("POST", root, token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID)
	var cursor *string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT cursor_url FROM email_outlook_folder WHERE tenant_id=$1 AND remote_id='inbox'", tenant.ID).Scan(&cursor))
	require.NotNil(t, cursor, "the Inbox synced and keeps its link")

	f.mu.Lock()
	failing = true
	f.mu.Unlock()
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_folder SET checked_at=now()-interval '5 minutes' WHERE tenant_id=$1 AND remote_id='inbox'", tenant.ID)
	require.NoError(t, err)
	runPostponedCalendarJobs(t, a, tenant.ID)
	var mail api.OutlookMailPage
	a.request("GET", root, token, nil).JSON(&mail)
	message, err := mail.Error.Get()
	require.NoError(t, err, "the Inbox failing is said")
	assert.Contains(t, message, "Microsoft said SyncStateInvalid")
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT cursor_url FROM email_outlook_folder WHERE tenant_id=$1 AND remote_id='inbox'", tenant.ID).Scan(&cursor))
	assert.Nil(t, cursor, "the rejected link is dropped: the Inbox starts afresh")
}

// A message Outlook lists without a change date is kept (its received date
// stands in), and one with no date at all is left out: neither holds up its
// folder, which used to retry the same page for ever ("Outlook could not
// sync").
func TestABadMessageDoesNotHoldUpItsFolder(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := &outlookMailFixture{folder: "inbox"}
	connectTestOutlook(t, a, token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "inbox") && strings.HasSuffix(r.URL.Path, "/messages/delta") {
			w.Header().Set("Content-Type", "application/json")
			now := time.Now().UTC()
			if r.URL.Query().Has("$skiptoken") { // the second page: a message with no change date, one with no date at all
				_ = json.NewEncoder(w).Encode(map[string]any{
					"value": []any{
						map[string]any{"id": "no-change-date", "parentFolderId": "inbox", "subject": "Second", "receivedDateTime": now},
						map[string]any{"id": "no-date", "parentFolderId": "inbox", "subject": "Undated"},
					},
					// Microsoft may write the mailbox as /users/{id}: the same folder.
					"@odata.deltaLink": "http://" + r.Host + "/users/abc-123/mailFolders('inbox')/messages/delta?$deltatoken=three",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value":           []any{map[string]any{"id": "fine", "parentFolderId": "inbox", "subject": "First", "receivedDateTime": now, "lastModifiedDateTime": now}},
				"@odata.nextLink": "http://" + r.Host + "/me/mailFolders/inbox/messages/delta?$skiptoken=two",
			})
			return
		}
		f.serve(w, r)
	}))
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID)
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_folder SET checked_at=now()-interval '20 minutes' WHERE tenant_id=$1 AND remote_id='inbox'", tenant.ID)
	require.NoError(t, err)
	runPostponedCalendarJobs(t, a, tenant.ID)
	var cursor *string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT cursor_url FROM email_outlook_folder WHERE tenant_id=$1 AND remote_id='inbox'", tenant.ID).Scan(&cursor))
	require.NotNil(t, cursor)
	assert.Contains(t, *cursor, "deltatoken=three", "the folder got through both pages")
	var mail api.OutlookMailPage
	a.request("GET", "/v1/outlook/mail", token, nil).JSON(&mail)
	assert.True(t, mail.Error.IsNull(), "no 'could not sync': %v", mail.Error)
	var subjects []string
	for _, m := range mail.Items {
		subjects = append(subjects, m.Subject)
	}
	assert.ElementsMatch(t, []string{"First", "Second"}, subjects, "the undated one is left out")
}
