package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra/entratest"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// serverCalendarApp is the server's Microsoft app (the one msTest signs in
// through), lent to the calendar as the emails module would.
type serverCalendarApp struct{}

func (serverCalendarApp) MicrosoftApp(context.Context, pgx.Tx, string) (calendars.Application, error) {
	return calendars.Application{ClientID: "server-client", ClientSecret: "server-secret"}, nil
}

// outlookSignIn is msTest with Microsoft Graph faked for mail (whose
// mailbox the tokens are) and the calendar (outlookFixture).
type outlookSignIn struct {
	*msTest
	calendar *outlookFixture
	mu       sync.Mutex
	mailbox  string // the address Graph says the tokens are for
}

func newOutlookSignIn(t *testing.T) *outlookSignIn {
	t.Helper()
	o := &outlookSignIn{msTest: newMicrosoftTest(t)}
	mail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/profile" || r.Header.Get("Authorization") != "Bearer graph-access" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		o.mu.Lock()
		defer o.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "acct-" + o.mailbox, "mail": o.mailbox})
	}))
	t.Cleanup(mail.Close)
	o.a.modules.Emails.ConnectMailboxesWith(emails.ConnectionSettings{KeysSecret: []byte("synthetic-mailbox-encryption-key-32-chars"),
		PublicURL: "http://app.test", ServerMode: "test", AllowPrivate: true, HTTPClient: mail.Client(),
		Microsoft: emails.OAuthApplication{ClientID: "server-client", ClientSecret: "server-secret"},
		Providers: map[string]emails.OAuthProvider{"microsoft": {AuthorizeURL: mail.URL + "/authorize", TokenURL: mail.URL + "/token", ProfileURL: mail.URL + "/profile", SendURL: mail.URL + "/sendMail", Scope: "send"}}})
	o.calendar = &outlookFixture{events: map[string]map[string]any{}}
	graph := httptest.NewServer(http.HandlerFunc(o.calendar.serve))
	t.Cleanup(graph.Close)
	o.a.modules.Calendars.ConnectOutlookWith(calendars.Deps{Apps: serverCalendarApp{}}, calendars.Config{
		KeysSecret: []byte("synthetic-calendar-key-01234567890123456789"), PublicURL: "http://app.test", HTTP: graph.Client(),
		Provider: calendars.Provider{AuthorizeURL: graph.URL + "/authorize", TokenURL: graph.URL + "/token", GraphURL: graph.URL}})
	return o
}

const outlookGranted = "openid profile email offline_access User.Read Mail.Send Mail.ReadWrite Calendars.ReadWrite"

// signIn signs acc in through the company's sign-in page, Microsoft granting
// Outlook too when the sign-in asked for it; it returns the session.
func (o *outlookSignIn) signIn(slug string, acc account) (string, msFlow) {
	o.t.Helper()
	flow, res := o.start(map[string]any{"company": slug})
	require.Equal(o.t, http.StatusOK, res.Status, "%s", res.Body)
	code := "code-" + randomBinding()
	g := entratest.Grant{Claims: acc.claims(flow)}
	if strings.Contains(flow.scope, "Mail.Send") {
		g.AccessToken, g.RefreshToken, g.Scope = "graph-access", "graph-refresh", outlookGranted
	}
	o.ms.Grant(code, g)
	return session(o.t, o.complete(o.ticket(flow, code), flow.binding, "")), flow
}

func mailboxOf(t *testing.T, a *testApp, token string) api.MailboxStatus {
	t.Helper()
	var s api.MailboxStatus
	res := a.request(http.MethodGet, "/v1/mailbox", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&s)
	return s
}

func calendarOf(t *testing.T, a *testApp, token string) api.OutlookConnection {
	t.Helper()
	var c api.OutlookConnection
	res := a.request(http.MethodGet, "/v1/calendar/connection", token, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&c)
	return c
}

// A company that connects Outlook at sign-in asks Microsoft once, at
// sign-in, for the person's mail and calendar too: signing in connects both.
// Signing in again later (another account's mailbox meanwhile chosen) leaves
// what they chose; while it is off, sign-in asks only who they are.
func TestMicrosoftSignInConnectsOutlook(t *testing.T) {
	t.Parallel()
	o := newOutlookSignIn(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner := tenant.Users[api.RoleOwner].Token
	org := newGUID()
	o.enable(owner, org, false)
	slug := o.me(owner).Tenant.CareersSlug
	email := emailOf(api.RoleRecruiter, tenant.ID)
	sam := newAccount(org, email)

	// Off (as it starts): who they are, nothing more.
	token, flow := o.signIn(slug, sam)
	assert.Equal(t, "openid profile email", flow.scope)
	assert.False(t, mailboxOf(t, o.a, token).Connected)

	res := o.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner, map[string]any{"enabled": true, "organisationId": org, "required": false, "connectsOutlook": true})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	var settings api.MicrosoftSignInSettings
	res.JSON(&settings)
	require.True(t, settings.ConnectsOutlook)
	res = o.a.request(http.MethodPut, "/v1/agency/microsoft-sign-in", owner, map[string]any{"enabled": true, "organisationId": org, "required": false})
	res.JSON(&settings)
	require.True(t, settings.ConnectsOutlook, "kept when left out")

	o.mu.Lock()
	o.mailbox = email
	o.mu.Unlock()
	token, flow = o.signIn(slug, sam)
	for _, scope := range []string{"Mail.Send", "Mail.ReadWrite", "Calendars.ReadWrite", "offline_access"} {
		assert.Contains(t, flow.scope, scope, "one consent: Outlook asked for at sign-in")
	}
	exchanges := o.ms.Exchanges()
	assert.Contains(t, exchanges[len(exchanges)-1].Scope, "Mail.Send", "the code redeemed for the same scopes")
	box := mailboxOf(t, o.a, token)
	require.True(t, box.Connected, "the mailbox is connected at sign-in")
	require.NotNil(t, box.Address)
	assert.Equal(t, email, string(*box.Address))
	assert.True(t, calendarOf(t, o.a, token).Connected, "and the calendar")
	assert.Len(t, audited(t, tenant.ID, "mailbox.connected"), 1)
	assert.Contains(t, audited(t, tenant.ID, "mailbox.connected")[0], "microsoft_sign_in")

	// Signed in again: the same mailbox is refreshed, not connected twice.
	token, _ = o.signIn(slug, sam)
	assert.True(t, mailboxOf(t, o.a, token).Connected)
	assert.Len(t, audited(t, tenant.ID, "mailbox.connected"), 1)
	assert.Len(t, audited(t, tenant.ID, "mailbox.reconnected"), 1)

	// Their mailbox is another account's now: a sign-in leaves it.
	o.mu.Lock()
	o.mailbox = "shared-desk@" + strings.Split(email, "@")[1]
	o.mu.Unlock()
	token, _ = o.signIn(slug, sam)
	box = mailboxOf(t, o.a, token)
	require.NotNil(t, box.Address)
	assert.Equal(t, email, string(*box.Address), "a mailbox of another account is not replaced")
}
