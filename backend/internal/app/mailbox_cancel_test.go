package app_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestMailboxDisconnectCancelsOAuthAlreadyExchangingItsCode(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	token := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			// Cancellation after the state was claimed but before tokens were received.
			require.Equal(t, 204, a.request("DELETE", "/v1/mailbox", token, nil).Status)
			_, _ = w.Write([]byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","token_type":"Bearer","expires_in":3600}`))
			return
		}
		_, _ = w.Write([]byte(`{"email":"sender@example.test","email_verified":true}`))
	}))
	defer provider.Close()
	a.modules.Emails.ConnectMailboxesWith(emails.ConnectionSettings{KeysSecret: []byte("synthetic-mailbox-key-longer-than-32-characters"), PublicURL: "http://app.test", Google: emails.OAuthApplication{ClientID: "synthetic-client", ClientSecret: "synthetic-secret"}, HTTPClient: provider.Client(), Providers: map[string]emails.OAuthProvider{"google": {AuthorizeURL: provider.URL + "/authorize", TokenURL: provider.URL + "/token", ProfileURL: provider.URL + "/profile"}}})
	r := a.request("POST", "/v1/mailbox/authorize", token, map[string]any{"provider": "google"})
	require.Equal(t, 200, r.Status)
	var auth api.MailboxAuthorization
	r.JSON(&auth)
	target, err := url.Parse(auth.Url)
	require.NoError(t, err)
	callback := "/v1/mailbox/callback?" + url.Values{"state": {target.Query().Get("state")}, "code": {"synthetic-code"}}.Encode()
	r = a.request("GET", callback, "", nil)
	require.Equal(t, 303, r.Status)
	r = a.request("POST", "/v1/mailbox/complete", token, map[string]any{"ticket": mailboxTicket(t, r.Header.Get("Location"))})
	assert.Equal(t, 400, r.Status, "disconnected while the code was being exchanged: %s", r.Body)
	var status api.MailboxStatus
	a.request("GET", "/v1/mailbox", token, nil).JSON(&status)
	assert.False(t, status.Connected)
}

// mailboxTicket is the one-time ticket in the address the provider's
// redirect sends the browser back to.
func mailboxTicket(t *testing.T, location string) string {
	t.Helper()
	require.Contains(t, location, "mailbox=confirm&ticket=", "the redirect parks the sign-in and hands over a ticket")
	return location[strings.Index(location, "ticket=")+len("ticket="):]
}
