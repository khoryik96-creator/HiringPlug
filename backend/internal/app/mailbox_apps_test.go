package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// The agency's own Microsoft app, entered in Settings instead of the
// server's settings file: kept sealed and masked, used to connect and to
// refresh mailboxes, and the server's used again once it is removed.
func TestMailAppsEnteredInSettings(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			exchanges.Add(1)
			assert.Equal(t, "agency-client-id", r.Form.Get("client_id"), "the agency's app, not the server's")
			assert.Equal(t, "agency-secret-9f2a", r.Form.Get("client_secret"))
			_, _ = w.Write([]byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","token_type":"Bearer","expires_in":3600}`))
		case "/profile":
			_, _ = w.Write([]byte(`{"mail":"recruiter@example.test"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	endpoint := emails.OAuthProvider{AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile", SendURL: server.URL + "/sendMail", Scope: "send email"}
	a.modules.Emails.ConnectMailboxesWith(emails.ConnectionSettings{KeysSecret: []byte("synthetic-mailbox-encryption-key-32-chars"), PublicURL: "http://app.test",
		AllowPrivate: true, HTTPClient: server.Client(), Google: emails.OAuthApplication{ClientID: "server-google", ClientSecret: "server-google-secret"},
		Providers: map[string]emails.OAuthProvider{"microsoft": endpoint}})

	var apps api.MailApps
	r := a.request(http.MethodGet, "/v1/settings/email-apps", owner, nil)
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	r.JSON(&apps)
	assert.True(t, apps.StorageReady)
	assert.Equal(t, "http://app.test/v1/mailbox/callback", apps.CallbackUrl, "the redirect to register")
	assert.Equal(t, api.MailAppSourceNone, apps.Microsoft.Source)
	assert.Equal(t, api.MailAppSourceServer, apps.Google.Source)
	assert.Equal(t, "server-google", apps.Google.ClientId.MustGet())
	assert.NotContains(t, string(r.Body), "server-google-secret", "a secret is never shown")
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/settings/email-apps", recruiter, nil).Status)

	var status api.MailboxStatus
	a.request(http.MethodGet, "/v1/mailbox", recruiter, nil).JSON(&status)
	assert.False(t, status.MicrosoftReady, "no Microsoft app yet")

	res := a.request(http.MethodPut, "/v1/settings/email-apps/microsoft", owner, map[string]any{"clientId": "agency-client-id"})
	require.Equal(t, http.StatusBadRequest, res.Status, "a new app needs its secret: %s", res.Body)
	res = a.request(http.MethodPut, "/v1/settings/email-apps/microsoft", owner, map[string]any{"clientId": " agency-client-id ", "clientSecret": "agency-secret-9f2a"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&apps)
	assert.Equal(t, api.MailAppSourceAgency, apps.Microsoft.Source)
	assert.Equal(t, "agency-client-id", apps.Microsoft.ClientId.MustGet())
	assert.Equal(t, "9f2a", apps.Microsoft.SecretEnding.MustGet(), "only the last four characters")
	assert.NotContains(t, string(res.Body), "agency-secret")
	var stored string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT sealed_secret FROM email_mail_app WHERE tenant_id=$1", tenant.ID).Scan(&stored))
	assert.NotContains(t, stored, "agency-secret", "sealed, never in the clear")

	// Keeping the secret: only the client ID sent.
	res = a.request(http.MethodPut, "/v1/settings/email-apps/microsoft", owner, map[string]any{"clientId": "agency-client-id"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	a.request(http.MethodGet, "/v1/mailbox", recruiter, nil).JSON(&status)
	assert.True(t, status.MicrosoftReady)
	r = a.request(http.MethodPost, "/v1/mailbox/authorize", recruiter, map[string]any{"provider": "microsoft"})
	require.Equal(t, http.StatusOK, r.Status, "%s", r.Body)
	var authorization api.MailboxAuthorization
	r.JSON(&authorization)
	target, err := url.Parse(authorization.Url)
	require.NoError(t, err)
	assert.Equal(t, "agency-client-id", target.Query().Get("client_id"))
	callback := "/v1/mailbox/callback?" + url.Values{"state": {target.Query().Get("state")}, "code": {"synthetic-code"}}.Encode()
	r = a.request(http.MethodGet, callback, "", nil)
	assert.True(t, strings.Contains(r.Header.Get("Location"), "mailbox=connected"), "%s", r.Header.Get("Location"))
	assert.EqualValues(t, 1, exchanges.Load(), "the code was exchanged with the agency's app")

	// Another agency does not get this one's app.
	other := testutil.Current.NewTenant(t, api.RoleOwner)
	var theirs api.MailApps
	a.request(http.MethodGet, "/v1/settings/email-apps", other.Users[api.RoleOwner].Token, nil).JSON(&theirs)
	assert.Equal(t, api.MailAppSourceNone, theirs.Microsoft.Source)

	res = a.request(http.MethodDelete, "/v1/settings/email-apps/microsoft", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&apps)
	assert.Equal(t, api.MailAppSourceNone, apps.Microsoft.Source, "removed: back to the server's, which has none")
}
