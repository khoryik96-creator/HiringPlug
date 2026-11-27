package app_test

import (
	"context"
	"encoding/base64"
	"io"
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
	f := newMicrosoftTest(t, liveServerMail())
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
	var inv api.Invitation
	res.JSON(&inv)
	assert.Equal(t, api.InvitationDeliveryServer, inv.Delivery, "the server's mail is set up: it goes from there")
	assert.True(t, inv.Emailed)
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

// liveServerMail sets up the server's own mail (HP_SMTP_*), as on a real
// server: the test mail server, which is not a test inbox's address.
func liveServerMail() map[string]string {
	return map[string]string{"HP_SMTP_HOST": mailServer.Addr, "HP_SMTP_FROM": "no-reply@agency.test", "HP_SMTP_TLS": "none"}
}

// Without the server's own mail (HP_SMTP_* not set up), an invitation goes
// from the inviter's connected mailbox (they are shown its link anyway), and
// says so; with no mailbox either, it says it was not emailed. Other account
// emails (a link to connect Microsoft) still never go through a person's
// mailbox.
func TestInvitationsGoFromTheInvitersMailboxWithoutServerMail(t *testing.T) {
	t.Parallel()
	f := newMicrosoftTest(t) // no HP_SMTP_HOST: the server's mail is not set up
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleAdmin, api.RoleRecruiter)
	owner, admin, recruiter := tenant.Users[api.RoleOwner], tenant.Users[api.RoleAdmin], tenant.Users[api.RoleRecruiter]
	f.enable(owner.Token, newGUID(), false)

	invite := func(token, email string) api.Invitation {
		t.Helper()
		res := f.a.request(http.MethodPost, "/v1/team/invites", token, map[string]any{"email": email, "name": "Pat", "role": "recruiter"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
		var inv api.Invitation
		res.JSON(&inv)
		return inv
	}

	// The admin has no mailbox: nothing can send it, and the answer says so.
	none := invite(admin.Token, "sam."+tenant.ID+"@test.local")
	assert.Equal(t, api.InvitationDeliveryNone, none.Delivery)
	assert.False(t, none.Emailed, "not on its way: pass the link on another way")
	assert.NotEmpty(t, none.Link)

	// The owner's Outlook is connected: the invitation goes through it.
	outlook := &outlookMailFixture{folder: "inbox"}
	var sent atomic.Int32
	var sentBody atomic.Value
	connectTestOutlook(t, f.a, owner.Token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMail") {
			sent.Add(1)
			b, _ := io.ReadAll(r.Body)
			sentBody.Store(string(b))
			w.WriteHeader(http.StatusAccepted)
			return
		}
		outlook.serve(w, r)
	}))
	inv := invite(owner.Token, "pat."+tenant.ID+"@test.local")
	assert.Equal(t, api.InvitationDeliveryMailbox, inv.Delivery)
	assert.True(t, inv.Emailed)
	runCoreJobsUntilIdle(t, f.a, tenant.ID)
	require.EqualValues(t, 1, sent.Load(), "the invitation went through the owner's Outlook")
	raw, _ := sentBody.Load().(string)
	mime, err := base64.StdEncoding.DecodeString(raw) // Outlook is sent the message as MIME, in base64
	require.NoError(t, err)
	body := strings.ReplaceAll(string(mime), "=\r\n", "") // its quoted-printable lines joined
	assert.Contains(t, body, "invited you")
	assert.Contains(t, body, inv.Link, "with its link")

	// A link to connect Microsoft is never handed to whoever sends it.
	f.reconnectLink(owner.Token, recruiter.ID)
	runCoreJobsUntilIdle(t, f.a, tenant.ID)
	assert.EqualValues(t, 1, sent.Load(), "the reconnect link did not go through the owner's Outlook")
}

// The Team page says how each invitation email went: sent, or failed with the
// mail server's answer (it used to say "emailed" for one the server refused).
func TestTheTeamPageSaysHowEachInvitationWent(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, liveServerMail())
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	for _, email := range []string{"pat." + tenant.ID + "@test.local", "refused." + tenant.ID + "@test.local"} {
		res := a.request(http.MethodPost, "/v1/team/invites", owner, map[string]any{"email": email, "name": "Pat", "role": "recruiter"})
		require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	}
	var queued api.TeamList
	a.request(http.MethodGet, "/v1/team", owner, nil).JSON(&queued)
	for _, m := range queued.Items {
		if m.Status == api.TeamStatusInvited {
			require.NotNil(t, m.InvitationEmail, m.Email)
			assert.Equal(t, api.EmailStatusQueued, m.InvitationEmail.Status, "waiting to be sent: %s", m.Email)
		}
	}
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var team api.TeamList
	a.request(http.MethodGet, "/v1/team", owner, nil).JSON(&team)
	states := map[string]api.InvitationEmail{}
	for _, m := range team.Items {
		if m.InvitationEmail != nil {
			states[strings.SplitN(m.Email, ".", 2)[0]] = *m.InvitationEmail
		}
	}
	require.Len(t, states, 2, "the owner, active, has none")
	assert.Equal(t, api.EmailStatusSent, states["pat"].Status)
	assert.True(t, states["pat"].Error.IsNull())
	assert.Equal(t, api.EmailStatusFailed, states["refused"].Status)
	assert.Contains(t, states["refused"].Error.MustGet(), "550", "with the mail server's answer")
}
