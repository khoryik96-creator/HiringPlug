package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type replyDraftProvider struct {
	*draftProvider
	requests []string
}

func (f *replyDraftProvider) serve(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/me/messages/remote-message/")
	if r.Method != http.MethodPost || (action != "createReply" && action != "createReplyAll") {
		f.draftProvider.serve(w, r)
		return
	}
	if r.ContentLength != 0 || !strings.Contains(r.Header.Get("Prefer"), `IdType="ImmutableId"`) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, action)
	to := []any{outlookRecipientJSON("nadia@example.test")}
	if action == "createReplyAll" {
		to = append(to, outlookRecipientJSON("lina@example.test"))
	}
	id := fmt.Sprintf("reply-draft-%d", f.sequence)
	body := map[string]any{
		"id": id, "parentFolderId": "drafts", "conversationId": "thread-one", "subject": "RE: Availability for the interview",
		"toRecipients": to, "isDraft": true, "createdDateTime": time.Now().UTC(),
		"body":    map[string]string{"contentType": "html", "content": "<p><br></p><blockquote>I can make Tuesday.</blockquote>"},
		"webLink": "https://outlook.office.com/mail/drafts/id",
	}
	f.updateVersion(body)
	f.drafts[id] = body
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(body)
}

func TestOutlookReplyDraftsUseTheOriginalConversationAndRecipients(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	user, other := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	f := &replyDraftProvider{draftProvider: newDraftProvider()}
	connectTestOutlook(t, a, user, http.HandlerFunc(f.serve))
	require.Equal(t, http.StatusNoContent, a.request("POST", "/v1/outlook/mail", user, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var listed api.OutlookMailPage
	a.request("GET", "/v1/outlook/mail", user, nil).JSON(&listed)
	require.Len(t, listed.Items, 1)
	path := "/v1/outlook/messages/" + listed.Items[0].Id
	replyPath := path + "/reply-draft"
	res := a.request("POST", replyPath, other, map[string]bool{"replyAll": false})
	require.Equal(t, http.StatusConflict, res.Status)

	candidate := a.createCandidate(user, map[string]any{"firstName": "Nadia", "email": "nadia@example.test"})
	require.Equal(t, http.StatusNoContent, a.request("PUT", path+"/link", user, map[string]string{"candidateId": candidate.Id}).Status)

	replies := make([]api.OutlookDraft, 0, 2)
	for _, replyAll := range []bool{false, true} {
		res = a.request("POST", replyPath, user, map[string]bool{"replyAll": replyAll})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var draft api.OutlookDraft
		res.JSON(&draft)
		require.NotNil(t, draft.BodyHtml)
		assert.Contains(t, *draft.BodyHtml, "<blockquote>I can make Tuesday.</blockquote>")
		replies = append(replies, draft)
	}
	assert.Equal(t, []string{"nadia@example.test"}, replies[0].To)
	assert.Equal(t, []string{"nadia@example.test", "lina@example.test"}, replies[1].To)
	assert.NotEqual(t, replies[0].MessageId, replies[1].MessageId)

	draft := replies[1]
	res = a.request("PUT", "/v1/outlook/drafts/"+draft.MessageId+"/send", user, map[string]any{
		"draftVersion": draft.Version, "expectedVersion": 0, "sendAt": time.Now().Add(time.Hour),
		"changes": map[string]string{"bodyHtml": "<p><b>Tuesday works.</b></p><blockquote>I can make Tuesday.</blockquote>"},
	})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)

	a.setConsent(user, candidate.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	res = a.request("POST", replyPath, user, map[string]bool{"replyAll": false})
	assert.Contains(t, string(res.Body), "EMAIL_DO_NOT_CONTACT")
	f.mu.Lock()
	requests := append([]string(nil), f.requests...)
	attempts := f.attempts
	f.mu.Unlock()
	assert.Equal(t, []string{"createReply", "createReplyAll"}, requests)
	assert.Zero(t, attempts, "creating a reply draft never sends email")
}
