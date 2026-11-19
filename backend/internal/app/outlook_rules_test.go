package app_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type outlookRulesFixture struct {
	mail *outlookMailFixture
	mu   sync.Mutex
	rule map[string]any
}

func newOutlookRulesFixture() *outlookRulesFixture {
	return &outlookRulesFixture{mail: &outlookMailFixture{folder: "inbox"}}
}

func (f *outlookRulesFixture) serve(w http.ResponseWriter, r *http.Request) {
	const root = "/me/mailFolders/inbox/messageRules"
	write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	if r.URL.Path == "/token" && strings.HasPrefix(r.FormValue("code"), "rules-") {
		scopes := "Mail.ReadWrite Mail.Send MailboxSettings.ReadWrite"
		if r.FormValue("code") == "rules-read-only" {
			scopes = "Mail.ReadWrite Mail.Send MailboxSettings.Read"
		}
		write(map[string]any{
			"access_token": r.FormValue("code"), "refresh_token": "synthetic-renewal", "expires_in": 3600,
			"token_type": "Bearer", "scope": scopes,
		})
		return
	}
	if r.URL.Path == "/me" {
		account := "recruiter-microsoft-id"
		if r.Header.Get("Authorization") == "Bearer rules-other" {
			account = "other-microsoft-id"
		}
		write(map[string]string{"id": account})
		return
	}
	if !strings.HasPrefix(r.URL.Path, root) {
		f.mail.serve(w, r)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer rules-") {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	readOnly := map[string]any{"id": "native-read-only", "displayName": "Outlook built-in", "sequence": 3, "isReadOnly": true, "isEnabled": true}
	switch {
	case r.URL.Path == root && r.Method == http.MethodGet:
		items := []any{readOnly}
		if f.rule != nil {
			items = append(items, f.rule)
		}
		write(map[string]any{"value": items})
	case r.URL.Path == root && r.Method == http.MethodPost:
		if err := json.NewDecoder(r.Body).Decode(&f.rule); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.rule["id"] = "rule-native"
		w.WriteHeader(http.StatusCreated)
		write(f.rule)
	case r.URL.Path == root+"/native-read-only" && r.Method == http.MethodGet:
		write(readOnly)
	case r.URL.Path == root+"/rule-native" && f.rule != nil:
		switch r.Method {
		case http.MethodGet:
			write(f.rule)
		case http.MethodPatch:
			var changes map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&changes); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.rule["isEnabled"] = changes["isEnabled"]
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			f.rule = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func authorizeInboxRules(t *testing.T, a *testApp, token, code string) *response {
	t.Helper()
	r := a.request("POST", "/v1/mailbox/authorize", token, map[string]any{"provider": "microsoft", "inboxRules": true})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var start api.MailboxAuthorization
	r.JSON(&start)
	target, err := url.Parse(start.Url)
	require.NoError(t, err)
	require.Contains(t, strings.Fields(target.Query().Get("scope")), "MailboxSettings.ReadWrite")
	return finishOutlookFixture(t, a, token, target, code)
}

func TestNativeOutlookRulesKeepTheExistingMailboxAndFolders(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	recruiter, coworker := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
	setupOutlookMail(t, a, recruiter)
	standard := "/v1/outlook/mail"
	require.Equal(t, 204, a.request("POST", standard, recruiter, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var before api.OutlookMailPage
	a.request("GET", standard, recruiter, nil).JSON(&before)
	require.Len(t, before.Items, 1)
	f := newOutlookRulesFixture()
	configureOutlookFixture(t, a, http.HandlerFunc(f.serve))

	root := "/v1/outlook/rules"
	r := a.request("GET", root, recruiter, nil)
	require.Equal(t, 403, r.Status, "%s", r.Body)
	assert.Equal(t, api.ErrorCodeOUTLOOKRULESACCESS, r.Error().Error.Code)
	require.Equal(t, 409, a.request("GET", root, coworker, nil).Status)
	r = authorizeInboxRules(t, a, recruiter, "rules-other")
	require.Equal(t, 400, r.Status, "%s", r.Body)
	require.Equal(t, 403, a.request("GET", root, recruiter, nil).Status)
	r = authorizeInboxRules(t, a, recruiter, "rules-read-only")
	require.Equal(t, 400, r.Status, "%s", r.Body)
	require.Equal(t, 403, a.request("GET", root, recruiter, nil).Status)
	r = authorizeInboxRules(t, a, recruiter, "rules-same")
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var after api.OutlookMailPage
	a.request("GET", standard, recruiter, nil).JSON(&after)
	require.True(t, after.Enabled)
	require.Equal(t, before.Items, after.Items, "the same mailbox keeps its local index")

	r = a.request("GET", root, recruiter, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var list api.OutlookRuleList
	r.JSON(&list)
	require.Len(t, list.Items, 1)
	require.Len(t, list.Folders, 1)
	require.Equal(t, "Archive", list.Folders[0].Name)
	require.Equal(t, 400, a.request("DELETE", root+"/"+list.Items[0].Id, recruiter, nil).Status)

	input := map[string]any{"name": "Applications", "folderId": list.Folders[0].Id}
	require.Equal(t, 400, a.request("POST", root, recruiter, input).Status)
	input["fromAddress"] = "alerts@example.test"
	input["subjectContains"] = "New applicant"
	r = a.request("POST", root, recruiter, input)
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var saved api.OutlookRule
	r.JSON(&saved)
	assert.Equal(t, "Archive", saved.Destination)
	require.True(t, saved.Enabled)
	f.mu.Lock()
	payload := f.rule
	f.mu.Unlock()
	assert.Equal(t, float64(4), payload["sequence"])
	assert.Equal(t, map[string]any{"moveToFolder": "archive", "stopProcessingRules": true}, payload["actions"])
	assert.Equal(t, map[string]any{
		"fromAddresses":   []any{map[string]any{"emailAddress": map[string]any{"address": "alerts@example.test"}}},
		"subjectContains": []any{"New applicant"},
	}, payload["conditions"])
	require.Equal(t, 400, a.request("POST", root, recruiter, input).Status, "retries cannot create a rule with the same name")
	path := root + "/" + saved.Id
	require.Equal(t, 204, a.request("PATCH", path, recruiter, map[string]bool{"enabled": false}).Status)
	a.request("GET", root, recruiter, nil).JSON(&list)
	require.Len(t, list.Items, 2)
	assert.False(t, list.Items[1].Enabled)
	require.Equal(t, 204, a.request("DELETE", path, recruiter, nil).Status)
	require.Equal(t, 404, a.request("DELETE", path, recruiter, nil).Status)

	created := a.request("POST", root, recruiter, input)
	require.Equal(t, 201, created.Status)
	created.JSON(&saved)
	setupOutlookMail(t, a, recruiter)
	configureOutlookFixture(t, a, http.HandlerFunc(f.serve))
	require.Equal(t, 200, authorizeInboxRules(t, a, recruiter, "rules-same").Status)
	stale := root + "/" + saved.Id
	require.Equal(t, 404, a.request("PATCH", stale, recruiter, map[string]bool{"enabled": false}).Status)
	require.Equal(t, 404, a.request("DELETE", stale, recruiter, nil).Status)
	a.request("GET", root, recruiter, nil).JSON(&list)
	require.Len(t, list.Items, 2, "a stale tab does not delete the provider rule")
	assert.True(t, list.Items[1].Enabled)
}
