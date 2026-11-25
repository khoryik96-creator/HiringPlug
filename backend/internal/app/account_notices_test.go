package app_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Emails about someone's account (an invitation, a link to connect
// Microsoft, a Microsoft account linked) go from the server's mail, sent by
// nobody: never from the connected mailbox of whoever caused them, where a
// copy in their Sent Items would hand them the link (and the account).
func TestAccountNoticesNeverGoThroughAPersonsMailbox(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleRecruiter]
	f.enable(owner.Token, newGUID(), false)

	// The owner's Outlook is connected: their own emails go through it.
	outlook := &outlookMailFixture{folder: "inbox"}
	var sentThroughOutlook atomic.Int32
	connectTestOutlook(t, f.a, owner.Token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMail") {
			sentThroughOutlook.Add(1)
			w.WriteHeader(http.StatusAccepted)
			return
		}
		outlook.serve(w, r)
	}))
	res := f.a.request(http.MethodPost, "/v1/emails", owner.Token, map[string]any{"to": []string{"someone." + tenant.ID + "@different-domain.test"},
		"subject": "Their own email", "bodyHtml": "<p>Through their mailbox.</p>"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, f.a, tenant.ID)
	require.EqualValues(t, 1, sentThroughOutlook.Load(), "the fake mailbox works")

	// The owner invites someone, and sends the recruiter a reconnect link.
	invitee := "pat." + tenant.ID + "@test.local"
	res = f.a.request(http.MethodPost, "/v1/team/invites", owner.Token, map[string]any{"email": invitee, "name": "Pat", "role": "recruiter"})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	link := f.reconnectLink(owner.Token, recruiter.ID) // from the server's mail (the helper reads it there)
	assert.NotEmpty(t, link)
	assert.Contains(t, f.lastMailTo(tenant.ID, invitee), "invited you")
	assert.EqualValues(t, 1, sentThroughOutlook.Load(), "neither went through the owner's mailbox")

	rows, err := testutil.Current.Admin.Query(context.Background(),
		`SELECT sent_by, mailbox_ref IS NULL, status FROM email WHERE tenant_id = $1 AND $2 <> ALL (to_addresses) ORDER BY created_at`,
		tenant.ID, "someone."+tenant.ID+"@different-domain.test")
	require.NoError(t, err)
	defer rows.Close()
	n := 0
	for rows.Next() {
		var sentBy, status string
		var serverMail bool
		require.NoError(t, rows.Scan(&sentBy, &serverMail, &status))
		assert.Equal(t, "system", sentBy, "sent by nobody: in no one's sent mail")
		assert.True(t, serverMail, "from the server's mail")
		assert.Equal(t, "sent", status)
		n++
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, 2, n, "the invitation and the reconnect link")
}
