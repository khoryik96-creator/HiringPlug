package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// connectOAuthMailbox connects the user's mailbox at a fake provider.
func (a *testApp) connectOAuthMailbox(token, provider string, server *httptest.Server) {
	a.t.Helper()
	endpoint := emails.OAuthProvider{AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile",
		SendURL: server.URL + "/me/sendMail", Scope: "send email"}
	app := emails.OAuthApplication{ClientID: "test-client", ClientSecret: "test-client-secret"}
	a.modules.Emails.ConnectMailboxesWith(emails.ConnectionSettings{KeysSecret: []byte("synthetic-mailbox-encryption-key-32-chars"), PublicURL: "http://app.test",
		ServerMode: "test", AllowPrivate: true, HTTPClient: server.Client(), Microsoft: app, Google: app, Providers: map[string]emails.OAuthProvider{provider: endpoint}})
	r := a.request(http.MethodPost, "/v1/mailbox/authorize", token, map[string]any{"provider": provider})
	require.Equal(a.t, http.StatusOK, r.Status, "%s", r.Body)
	var authorization api.MailboxAuthorization
	r.JSON(&authorization)
	target, err := url.Parse(authorization.Url)
	require.NoError(a.t, err)
	r = a.request(http.MethodGet, "/v1/mailbox/callback?"+url.Values{"state": {target.Query().Get("state")}, "code": {"synthetic-code"}}.Encode(), "", nil)
	require.Contains(a.t, r.Header.Get("Location"), "mailbox=connected")
}

// fakeMailProvider is a provider's sign-in, profile, send and inbox.
func fakeMailProvider(t *testing.T, inbox func() string, reads *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","token_type":"Bearer","expires_in":3600}`))
		case "/profile":
			_, _ = w.Write([]byte(`{"email":"recruiter@example.test","email_verified":true,"mail":"recruiter@example.test"}`))
		case "/me/sendMail":
			w.WriteHeader(http.StatusAccepted)
		case "/me/mailFolders/inbox/messages":
			reads.Add(1)
			assert.Equal(t, "Bearer synthetic-access", r.Header.Get("Authorization"))
			assert.Contains(t, r.URL.Query().Get("$filter"), "receivedDateTime ge ")
			_, _ = w.Write([]byte(inbox()))
		default:
			t.Errorf("unexpected provider path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func makeBounceScansDue(t *testing.T, tenantID string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'email.bounce_scan' AND status = 'pending'`, tenantID)
	require.NoError(t, err)
}

func TestARealBounceInTheSendersOutlookMarksTheContactsMailboxAsGone(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	var reads atomic.Int32
	var notices atomic.Value
	notices.Store(`{"value":[]}`)
	a.connectOAuthMailbox(token, "microsoft", fakeMailProvider(t, func() string { return notices.Load().(string) }, &reads))

	company := a.createCompany(token, map[string]any{"name": "Maju Sdn Bhd"})
	ana := a.createContact(token, map[string]any{"companyId": company.Id, "firstName": "Ana", "email": "Ana@maju.test"})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Equal(t, api.MailboxCheckStatusAccepted, a.contact(token, ana.Id).MailboxCheck.MustGet().Status, "the server takes any address")

	r := a.request(http.MethodPost, "/v1/emails", token, map[string]any{"contactId": ana.Id, "to": []string{"ana@maju.test"}, "cc": []string{"joana@maju.test"},
		"subject": "Synthetic shortlist", "bodyHtml": "<p>Three candidates.</p>"})
	require.Equal(t, http.StatusAccepted, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var scans int
	require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'email.bounce_scan'`, tenant.ID).Scan(&scans))
	assert.Equal(t, 2, scans, "looked at twice after sending")

	// Nothing came back yet: nothing changes.
	makeBounceScansDue(t, tenant.ID)
	_, err := testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() + interval '1 hour' WHERE tenant_id = $1 AND kind = 'email.bounce_scan' AND status = 'pending'
		AND id = (SELECT id FROM job WHERE tenant_id = $1 AND kind = 'email.bounce_scan' ORDER BY run_after DESC LIMIT 1)`, tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.EqualValues(t, 1, reads.Load())
	assert.Empty(t, a.contactEmail(token, ana.Id).BouncedTo)

	// Then a notice for Ana's address (not a colleague's whose address contains it).
	notices.Store(`{"value":[
		{"subject":"Out of office","body":{"content":"ana@maju.test is away"}},
		{"subject":"Undeliverable: Synthetic shortlist","body":{"content":"<p>Delivery has failed to these recipients:</p><p><a href=\"mailto:ana@maju.test\">ana@maju.test</a></p><p>The email address couldn&#39;t be found.</p>"}}]}`)
	makeBounceScansDue(t, tenant.ID)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.EqualValues(t, 2, reads.Load())
	got := a.contactEmail(token, ana.Id)
	assert.Equal(t, []string{"ana@maju.test"}, got.BouncedTo)
	assert.False(t, got.BouncedAt.IsNull())
	check := a.contact(token, ana.Id).MailboxCheck.MustGet()
	assert.Equal(t, api.MailboxCheckStatusInvalid, check.Status, "the mailbox is gone, whatever the server said before")
	assert.Contains(t, check.Reason.MustGet(), "A real email to this address bounced")

	// The composer now warns, and the monthly re-check does not clear it.
	r = a.request(http.MethodPost, "/v1/contacts/address-check", token, map[string]any{"emails": []string{"ana@maju.test"}})
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	assert.Contains(t, string(r.Body), `"problem":"bounces"`)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE contact_mailbox_check SET checked_at = now() - interval '31 days' WHERE tenant_id = $1`, tenant.ID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET run_after = now() WHERE tenant_id = $1 AND kind = 'contact.check_mailbox' AND status = 'pending'`, tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Equal(t, api.MailboxCheckStatusInvalid, a.contact(token, ana.Id).MailboxCheck.MustGet().Status)
	assert.Len(t, a.deps.Mailboxes.(*fakeMailboxes).callsFor("ana@maju.test"), 1, "not asked again")

	// Another agency's contact with the same address is not touched.
	strangerTenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	stranger := strangerTenant.Users[api.RoleRecruiter].Token
	other := a.createContact(stranger, map[string]any{"companyId": a.createCompany(stranger, map[string]any{"name": "Elsewhere"}).Id,
		"firstName": "Ana", "email": "ana@maju.test"})
	runCoreJobsUntilIdle(t, a, strangerTenant.ID)
	assert.Equal(t, api.MailboxCheckStatusAccepted, a.contact(stranger, other.Id).MailboxCheck.MustGet().Status)
}

func TestEmailsSentFromAMailboxHiringPlugCannotReadAreNotScannedForBounces(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	var reads atomic.Int32
	a.connectOAuthMailbox(token, "google", fakeMailProvider(t, func() string { return `{"value":[]}` }, &reads))
	r := a.request(http.MethodPost, "/v1/emails", token, map[string]any{"to": []string{"ana@maju.test"}, "subject": "Synthetic", "bodyHtml": "<p>Hi.</p>"})
	require.Equal(t, http.StatusAccepted, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var scans int
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'email.bounce_scan'`, tenant.ID).Scan(&scans))
	assert.Zero(t, scans, "Gmail is connected to send only")
	assert.Zero(t, reads.Load())
}

// contactEmail returns the latest email to a contact.
func (a *testApp) contactEmail(token, contactID string) api.Email {
	a.t.Helper()
	r := a.request(http.MethodGet, "/v1/emails?contactId="+contactID, token, nil)
	require.Equal(a.t, http.StatusOK, r.Status, "%s", r.Body)
	var list api.EmailList
	r.JSON(&list)
	require.NotEmpty(a.t, list.Items)
	return list.Items[0]
}
