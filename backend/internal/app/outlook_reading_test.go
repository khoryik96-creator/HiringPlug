package app_test

import (
	"encoding/base64"
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

var (
	outlookLogo   = []byte("\x89PNG\r\n\x1a\nsynthetic-logo")
	outlookBanner = []byte("\xff\xd8\xff\xe0\x00\x10JFIFsynthetic-banner")
)

// richOutlookMail is the Outlook fixture with a formatted message: a
// signature logo sent with it, a tracking picture from the web, and its
// flag kept as Outlook keeps it.
func richOutlookMail(t *testing.T, f *outlookMailFixture) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		write := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.URL.Path == "/me/messages/remote-message" && r.Method == http.MethodGet:
			assert.Contains(nil, r.URL.Query().Get("$select"), "toRecipients")
			write(map[string]any{
				"body": map[string]string{"contentType": "html", "content": `<html><head><style>.sig{color:#7352d8}</style></head><body>` +
					`<p>Hi <b>Samuel</b>,</p><table><tr><td>Role</td><td>SME Engineer</td></tr></table>` +
					`<p class="sig"><img src="cid:logo@sig" alt="Hyppies"><img src="cid:banner@sig" alt="Banner"></p><img src="https://track.test/o.gif"><script>x()</script></body></html>`},
				"from":         map[string]any{"emailAddress": map[string]string{"name": "Leander Natasha", "address": "natasha@hyppies.test"}},
				"toRecipients": []any{map[string]any{"emailAddress": map[string]string{"name": "Samuel", "address": "samuel@client.test"}}},
				"ccRecipients": []any{map[string]any{"emailAddress": map[string]string{"name": "", "address": "henk@client.test"}}},
				"sentDateTime": "2026-11-19T06:49:00Z", "importance": "high",
			})
		case r.URL.Path == "/me/messages/remote-message/attachments" && strings.Contains(r.URL.Query().Get("$select"), "contentType"):
			// A banner some mail apps send unmarked, as a plain file.
			write(map[string]any{"value": []any{
				map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": "inline-logo", "name": "signature.png", "isInline": true, "size": len(outlookLogo), "contentType": "image/png"},
				map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": "sig-banner", "name": "image002.jpg", "isInline": false, "size": len(outlookBanner), "contentType": "application/octet-stream"},
				map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": "cv", "name": "CV.pdf", "isInline": false, "size": 100, "contentType": "application/pdf"},
			}})
		case r.URL.Path == "/me/messages/remote-message/attachments/sig-banner":
			write(map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": "sig-banner", "contentId": "banner@sig",
				"contentType": "application/octet-stream", "contentBytes": base64.StdEncoding.EncodeToString(outlookBanner)})
		case r.URL.Path == "/me/messages/remote-message/attachments/cv":
			t.Errorf("a file that is no picture is not fetched to show the message")
		case r.URL.Path == "/me/messages/remote-message/attachments/inline-logo":
			write(map[string]any{"@odata.type": "#microsoft.graph.fileAttachment", "id": "inline-logo", "contentId": "<logo@sig>",
				"contentType": "image/png", "contentBytes": base64.StdEncoding.EncodeToString(outlookLogo)})
		case r.URL.Path == "/me/messages/remote-message" && r.Method == http.MethodPatch:
			var body struct {
				Flag *struct{ FlagStatus string }
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Flag == nil {
				f.serve(w, r)
				return
			}
			f.mu.Lock()
			f.flag = body.Flag.FlagStatus
			f.mu.Unlock()
			write(map[string]any{"lastModifiedDateTime": time.Now().UTC()})
		default:
			f.serve(w, r)
		}
	}
}

func TestOutlookMailReadsLikeOutlook(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := &outlookMailFixture{folder: "inbox", flag: "flagged"}
	connectTestOutlook(t, a, token, richOutlookMail(t, f))
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	var mail api.OutlookMailPage
	a.request("GET", "/v1/outlook/mail", token, nil).JSON(&mail)
	require.Len(t, mail.Items, 1)
	item := mail.Items[0]
	assert.Equal(t, "Nadia Rahman", item.SenderName)
	assert.True(t, item.Flagged)
	unread := map[string]int{}
	kinds := []string{}
	for _, folder := range mail.Folders {
		unread[folder.Kind] = folder.Unread
		kinds = append(kinds, folder.Kind)
	}
	assert.Equal(t, 1, unread["inbox"], "the inbox counts its unread message")
	assert.Equal(t, []string{"inbox", "drafts", "sentitems", "deleteditems", "archive", "junkemail"}, kinds, "folders in Outlook's order")
	var found api.OutlookMailPage
	a.request("GET", "/v1/outlook/mail?search=nadia+rah", token, nil).JSON(&found)
	assert.Len(t, found.Items, 1, "found by the sender's name")

	// The message: who it is from and to, its formatting, its signature logo.
	var body api.OutlookMessageBody
	r := a.request("GET", "/v1/outlook/messages/"+item.Id, token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	r.JSON(&body)
	assert.Equal(t, api.OutlookPerson{Name: "Leander Natasha", Address: "natasha@hyppies.test"}, body.From.MustGet())
	assert.Equal(t, []api.OutlookPerson{{Name: "Samuel", Address: "samuel@client.test"}}, body.To)
	assert.Equal(t, []api.OutlookPerson{{Name: "", Address: "henk@client.test"}}, body.Cc)
	assert.Equal(t, api.OutlookMessageBodyImportanceHigh, body.Importance)
	html := body.Html.MustGet()
	assert.Contains(t, html, "<b>Samuel</b>")
	assert.Contains(t, html, "<td>SME Engineer</td>")
	assert.Contains(t, html, ".sig{color:#7352d8}")
	assert.Contains(t, html, `src="data:image/png;base64,`+base64.StdEncoding.EncodeToString(outlookLogo)+`"`, "the logo sent with it is shown")
	assert.Contains(t, html, `src="data:image/jpeg;base64,`+base64.StdEncoding.EncodeToString(outlookBanner)+`"`, "a banner sent unmarked, as a plain file, is shown too")
	assert.Contains(t, html, `data-hp-src="https://track.test/o.gif"`)
	assert.NotContains(t, html, "<script")
	assert.Equal(t, 1, body.BlockedImages)
	assert.True(t, strings.HasPrefix(body.Text, "Hi Samuel,"), body.Text)

	// Flagged and unflagged, in Outlook and here.
	path := "/v1/outlook/messages/" + item.Id
	require.Equal(t, 204, a.request("PATCH", path, token, map[string]bool{"flagged": false}).Status)
	f.mu.Lock()
	assert.Equal(t, "notFlagged", f.flag)
	f.mu.Unlock()
	a.request("GET", "/v1/outlook/mail", token, nil).JSON(&mail)
	assert.False(t, mail.Items[0].Flagged)
	assert.Equal(t, 400, a.request("PATCH", path, token, map[string]bool{"flagged": true, "isRead": true}).Status, "one change at a time")

	// Read: no longer counted.
	require.Equal(t, 204, a.request("PATCH", path, token, map[string]bool{"isRead": true}).Status)
	a.request("GET", "/v1/outlook/mail", token, nil).JSON(&mail)
	for _, folder := range mail.Folders {
		assert.Zero(t, folder.Unread, folder.Name)
	}
}
