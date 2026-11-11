package app_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func smtpMailboxBody(t *testing.T, address, server string) map[string]any {
	t.Helper()
	host, port, err := net.SplitHostPort(server)
	require.NoError(t, err)
	n, err := strconv.Atoi(port)
	require.NoError(t, err)
	return map[string]any{"expectedVersion": 0, "address": address, "host": host, "port": n, "tls": "none", "username": ""}
}
func TestMailboxSMTPIsolationAndActualWorkerDelivery(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleViewer)
	owner := tenant.Users[api.RoleOwner].Token
	otherUser := tenant.Users[api.RoleRecruiter].Token
	address := "sender." + tenant.ID + "@agency.test"
	recipient := "recipient." + tenant.ID + "@different-domain.test"
	body := smtpMailboxBody(t, address, mailServer.Addr)
	r := a.request("PUT", "/v1/mailbox", owner, body)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var status api.MailboxStatus
	r.JSON(&status)
	assert.True(t, status.Connected)
	assert.Equal(t, 1, status.Version)
	assert.NotContains(t, string(r.Body), "sealed")
	assert.NotContains(t, string(r.Body), "password")
	for _, token := range []string{otherUser, testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token} {
		var own api.MailboxStatus
		a.request("GET", "/v1/mailbox", token, nil).JSON(&own)
		assert.False(t, own.Connected)
	}
	assert.Equal(t, 403, a.request("GET", "/v1/mailbox", tenant.Users[api.RoleViewer].Token, nil).Status)
	// The database, not just the endpoint, hides another user's credential row.
	require.NoError(t, database.InTx(context.Background(), testutil.Current.App, authz.NewPrincipal(tenant.ID, "usr_other", "", "", api.RoleOwner), func(tx pgx.Tx) error {
		var count int
		err := tx.QueryRow(context.Background(), "SELECT count(*) FROM email_mailbox").Scan(&count)
		assert.Zero(t, count)
		return err
	}))
	r = a.request("POST", "/v1/emails", owner, map[string]any{"to": []string{recipient}, "subject": "Synthetic provider-neutral email", "bodyHtml": "<p>Different domains work.</p>", "timeZone": "Asia/Kuala_Lumpur"})
	require.Equal(t, 202, r.Status, "%s", r.Body)
	var email api.Email
	r.JSON(&email)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	messages := mailServer.To(recipient)
	require.Len(t, messages, 1)
	assert.Contains(t, messages[0].Data, address)
	// An unchanged connection can retain its password/secret; stale writes cannot.
	assert.Equal(t, 409, a.request("PUT", "/v1/mailbox", owner, body).Status)
	body["expectedVersion"] = 1
	require.Equal(t, 200, a.request("PUT", "/v1/mailbox", owner, body).Status)
}
func TestQueuedMailboxEmailDoesNotFallBackAfterDisconnect(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	body := smtpMailboxBody(t, "sender@agency.test", mailServer.Addr)
	require.Equal(t, 200, a.request("PUT", "/v1/mailbox", token, body).Status)
	recipient := "disconnected." + tenant.ID + "@different-domain.test"
	r := a.request("POST", "/v1/emails", token, map[string]any{"to": []string{recipient}, "subject": "Synthetic queued email", "bodyHtml": "<p>Must not use server fallback.</p>"})
	require.Equal(t, 202, r.Status)
	var email api.Email
	r.JSON(&email)
	require.Equal(t, 204, a.request("DELETE", "/v1/mailbox", token, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.Empty(t, mailServer.To(recipient))
	var state, reason string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT status,error FROM email WHERE id=$1", email.Id).Scan(&state, &reason))
	assert.Equal(t, "failed", state)
	assert.Contains(t, reason, "disconnected or replaced")
}
func TestSMTPSecretCannotBeReusedForAnotherServer(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	body := smtpMailboxBody(t, "sender@agency.test", mailServer.Addr)
	body["username"] = "sender"
	body["password"] = "synthetic-mailbox-password"
	r := a.request("PUT", "/v1/mailbox", token, body)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	var sealed string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT sealed_secret FROM email_mailbox WHERE tenant_id=$1", tenant.ID).Scan(&sealed))
	assert.NotContains(t, sealed, "synthetic-mailbox-password")
	body["expectedVersion"] = 1
	delete(body, "password")
	body["host"] = "another.example.test"
	r = a.request("PUT", "/v1/mailbox", token, body)
	assert.Equal(t, 400, r.Status)
	assert.Contains(t, string(r.Body), "password")
	var audit string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT coalesce(string_agg(changes::text,''),'') FROM audit_log WHERE tenant_id=$1", tenant.ID).Scan(&audit))
	assert.False(t, strings.Contains(audit, "synthetic-mailbox-password"))
}

func TestMailboxSMTPCannotReachPrivateNetworksWhenDisabled(t *testing.T) {
	t.Parallel()
	a := newTestApp(t, map[string]string{"HP_ALLOW_PRIVATE_NETWORKS": "false"})
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	token := tenant.Users[api.RoleOwner].Token
	body := smtpMailboxBody(t, "sender@example.test", mailServer.Addr)
	body["tls"] = "starttls"
	r := a.request("PUT", "/v1/mailbox", token, body)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	recipient := "blocked." + tenant.ID + "@example.test"
	r = a.request("POST", "/v1/emails", token, map[string]any{"to": []string{recipient}, "subject": "Synthetic blocked server", "bodyHtml": "<p>Must not reach a private server.</p>"})
	require.Equal(t, 202, r.Status)
	var email api.Email
	r.JSON(&email)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var status, reason string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT status,error FROM email WHERE id=$1", email.Id).Scan(&status, &reason))
	assert.Equal(t, "failed", status)
	assert.Contains(t, reason, "private network")
	assert.Empty(t, mailServer.To(recipient))
}

func TestAPIKeysCannotManagePersonalMailboxes(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	owner := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	r := a.request("POST", "/v1/api-keys", owner, map[string]any{"name": "Synthetic integration", "role": "recruiter"})
	require.Equal(t, 201, r.Status)
	var key api.ApiKeyCreated
	r.JSON(&key)
	assert.Equal(t, 403, a.request("GET", "/v1/mailbox", key.Key, nil).Status)
	assert.Equal(t, 403, a.request("DELETE", "/v1/mailbox", key.Key, nil).Status)
	assert.Equal(t, 403, a.request("PUT", "/v1/mailbox", key.Key, smtpMailboxBody(t, "sender@example.test", mailServer.Addr)).Status)
	assert.Equal(t, 403, a.request("POST", "/v1/mailbox/authorize", key.Key, map[string]any{"provider": "google"}).Status)
}
