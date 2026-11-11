package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	stdmail "net/mail"
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

func TestMailboxOAuthProvidersAndRefreshSurviveSendRetry(t *testing.T) {
	for _, provider := range []string{"microsoft", "google"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			a := newTestApp(t)
			tenant := testutil.Current.NewTenant(t, api.RoleOwner)
			token := tenant.Users[api.RoleOwner].Token
			var refreshes, sends atomic.Int32
			var challenge string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/token":
					require.NoError(t, r.ParseForm())
					if r.Form.Get("grant_type") == "authorization_code" {
						sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
						assert.Equal(t, challenge, base64.RawURLEncoding.EncodeToString(sum[:]))
						_, _ = w.Write([]byte(`{"access_token":"synthetic-access","refresh_token":"synthetic-refresh","token_type":"Bearer","expires_in":1}`))
					} else {
						refreshes.Add(1)
						assert.Equal(t, "synthetic-refresh", r.Form.Get("refresh_token"))
						_, _ = w.Write([]byte(`{"access_token":"synthetic-refreshed","refresh_token":"synthetic-rotated","token_type":"Bearer","expires_in":3600}`))
					}
				case "/profile":
					assert.Equal(t, "Bearer synthetic-access", r.Header.Get("Authorization"))
					_, _ = w.Write([]byte(`{"email":"recruiter@example.test","email_verified":true,"mail":"recruiter@example.test"}`))
				case "/sendMail":
					assert.Equal(t, "Bearer synthetic-refreshed", r.Header.Get("Authorization"))
					if sends.Add(1) == 1 {
						w.WriteHeader(503)
						return
					}
					var raw []byte
					var err error
					if provider == "google" {
						var body struct {
							Raw string `json:"raw"`
						}
						require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
						raw, err = base64.RawURLEncoding.DecodeString(body.Raw)
					} else {
						encoded, readErr := io.ReadAll(r.Body)
						require.NoError(t, readErr)
						raw, err = base64.StdEncoding.DecodeString(string(encoded))
					}
					require.NoError(t, err)
					message, err := stdmail.ReadMessage(strings.NewReader(string(raw)))
					require.NoError(t, err)
					assert.Contains(t, message.Header.Get("From"), "recruiter@example.test")
					assert.Equal(t, "copy@different-domain.test", message.Header.Get("Cc"))
					assert.Equal(t, "private@different-domain.test", message.Header.Get("Bcc"), "API transports derive Bcc recipients from MIME headers")
					w.WriteHeader(202)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			endpoint := emails.OAuthProvider{AuthorizeURL: server.URL + "/authorize", TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile", SendURL: server.URL + "/sendMail", Scope: "send email"}
			a.modules.Emails.ConnectMailboxesWith(emails.ConnectionSettings{KeysSecret: []byte("synthetic-mailbox-encryption-key-32-chars"), PublicURL: "http://app.test", ServerMode: "test", AllowPrivate: true, HTTPClient: server.Client(), Microsoft: emails.OAuthApplication{ClientID: "test-client", ClientSecret: "test-client-secret"}, Google: emails.OAuthApplication{ClientID: "test-client", ClientSecret: "test-client-secret"}, Providers: map[string]emails.OAuthProvider{provider: endpoint}})
			r := a.request("POST", "/v1/mailbox/authorize", token, map[string]any{"provider": provider})
			require.Equal(t, 200, r.Status, "%s", r.Body)
			var authorization api.MailboxAuthorization
			r.JSON(&authorization)
			target, err := url.Parse(authorization.Url)
			require.NoError(t, err)
			challenge = target.Query().Get("code_challenge")
			assert.NotEmpty(t, challenge)
			assert.Equal(t, "S256", target.Query().Get("code_challenge_method"))
			state := target.Query().Get("state")
			callback := "/v1/mailbox/callback?" + url.Values{"state": {state}, "code": {"synthetic-code"}}.Encode()
			r = a.request("GET", callback, "", nil)
			require.Equal(t, 303, r.Status)
			assert.Contains(t, r.Header.Get("Location"), "mailbox=connected")
			assert.Equal(t, "no-store", r.Header.Get("Cache-Control"))
			assert.Equal(t, "no-referrer", r.Header.Get("Referrer-Policy"))
			// The same state cannot overwrite the connection twice, and errors are never reflected.
			replay := a.request("GET", callback, "", nil)
			assert.Contains(t, replay.Header.Get("Location"), "mailbox=failed")
			assert.NotContains(t, string(replay.Body), "synthetic")
			var status api.MailboxStatus
			r = a.request("GET", "/v1/mailbox", token, nil)
			r.JSON(&status)
			require.True(t, status.Connected)
			assert.Equal(t, "recruiter@example.test", string(*status.Address))
			assert.NotContains(t, string(r.Body), "synthetic-access")
			assert.NotContains(t, string(r.Body), "synthetic-refresh")
			r = a.request("POST", "/v1/emails", token, map[string]any{"to": []string{"receiver@different-domain.test"}, "cc": []string{"copy@different-domain.test"}, "bcc": []string{"private@different-domain.test"}, "subject": "Synthetic OAuth send", "bodyHtml": "<p>Provider-neutral mail.</p>"})
			require.Equal(t, 202, r.Status)
			var email api.Email
			r.JSON(&email)
			runCoreJobsUntilIdle(t, a, tenant.ID)
			// Retry is due later; make only this synthetic job due without waiting.
			_, err = testutil.Current.Admin.Exec(context.Background(), "UPDATE job SET run_after=now() WHERE tenant_id=$1 AND status='pending' AND kind='email.send'", tenant.ID)
			require.NoError(t, err)
			runCoreJobsUntilIdle(t, a, tenant.ID)
			assert.EqualValues(t, 1, refreshes.Load(), "token rotation must survive a rolled-back failed send")
			var delivery string
			require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT status FROM email WHERE id=$1", email.Id).Scan(&delivery))
			assert.Equal(t, "sent", delivery)
			var encrypted string
			require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT sealed_secret FROM email_mailbox WHERE tenant_id=$1", tenant.ID).Scan(&encrypted))
			assert.False(t, strings.Contains(encrypted, "synthetic"))
		})
	}
}
