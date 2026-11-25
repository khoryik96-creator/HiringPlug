package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// Review of 1.47.0: a held email.send cancelled after a restore left its
// email "queued" for ever (only Outlook's scheduled sends had a dead-job
// handler), and so did a webhook delivery or a mailbox check; a cancelled
// morning email ended that user's morning emails for good. Cancelled, each
// says it was not done; the morning email carries on the next morning.
func TestCancellingHeldJobsShowsWhatWasNotDone(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	ctx := context.Background()
	tenant := testutil.Current.NewTenant(t, api.RoleOwner)
	owner := tenant.Users[api.RoleOwner].Token
	admin := testutil.Current.Admin
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM job WHERE tenant_id = $1`, tenant.ID)
	})

	res := a.request(http.MethodPost, "/v1/emails", owner, map[string]any{"to": []string{"aisyah." + tenant.ID + "@client.test"},
		"subject": "Shortlist", "bodyHtml": "<p>Three CVs for you.</p>"})
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	_, srv := newReceiver(t)
	res = a.request(http.MethodPost, "/v1/webhooks", owner, map[string]any{"url": srv.URL + "/hooks", "events": []string{"candidate.created"}})
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var hook api.WebhookCreated
	res.JSON(&hook)
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/webhooks/"+hook.Webhook.Id+"/deliveries", owner, nil).Status)
	company := a.createCompany(owner, map[string]any{"name": "Maybank"})
	contact := a.createContact(owner, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "email": "aisyah@maybank.com.my"})
	require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/contacts/"+contact.Id+"/mailbox-check", owner, nil).Status)
	res = a.request(http.MethodPut, "/v1/notifications/settings", owner, map[string]any{"morningEmail": true, "mutedKinds": []string{}})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)

	// The server is restored: they wait, held; the owner cancels them all.
	tag, err := admin.Exec(ctx, `UPDATE job SET status = 'held', held_at = now() WHERE tenant_id = $1 AND status = 'pending'
		AND kind IN ('email.send', 'webhook.deliver', 'contact.check_mailbox', 'notifications.digest')`, tenant.ID)
	require.NoError(t, err)
	require.EqualValues(t, 4, tag.RowsAffected())
	res = a.request(http.MethodPost, "/v1/settings/system/held", owner, map[string]any{"action": "cancel"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	var status, why string
	require.NoError(t, admin.QueryRow(ctx, `SELECT status, error FROM email WHERE tenant_id = $1`, tenant.ID).Scan(&status, &why))
	assert.Equal(t, "failed", status, "the email says it was not sent")
	assert.Equal(t, "Not sent: cancelled after the server was restored (it was waiting in the backup).", why)
	require.NoError(t, admin.QueryRow(ctx, `SELECT status, last_error FROM webhook_delivery WHERE tenant_id = $1`, tenant.ID).Scan(&status, &why))
	assert.Equal(t, "failed", status)
	assert.Contains(t, why, "cancelled after the server was restored")
	require.NoError(t, admin.QueryRow(ctx, `SELECT status, reason FROM contact_mailbox_check WHERE tenant_id = $1`, tenant.ID).Scan(&status, &why))
	assert.Equal(t, "unknown", status, "not checking for ever")
	assert.Contains(t, why, "Check it again")
	var next int
	require.NoError(t, admin.QueryRow(ctx, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'notifications.digest' AND status = 'pending'`,
		tenant.ID).Scan(&next))
	assert.Equal(t, 1, next, "the next morning's email is queued")
}
