package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// holdAndCancel holds a company's waiting jobs of kind, as a restore does,
// and cancels them from Settings > System, then lets the worker act on it.
func holdAndCancel(t *testing.T, a *testApp, tenantID, owner string, kind api.JobKind) {
	t.Helper()
	tag, err := testutil.Current.Admin.Exec(context.Background(), `UPDATE job SET status = 'held', held_at = now()
		WHERE tenant_id = $1 AND status = 'pending' AND kind = $2`, tenantID, string(kind))
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected(), "one run of the chain waits: %s", kind)
	res := a.request(http.MethodPost, "/v1/settings/system/held", owner, map[string]any{"action": "cancel"})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenantID)
}

// waitingRuns is when a company's waiting runs of kind are due.
func waitingRuns(t *testing.T, tenantID string, kind api.JobKind) []time.Time {
	t.Helper()
	rows, err := testutil.Current.Admin.Query(context.Background(), `SELECT run_after FROM job
		WHERE tenant_id = $1 AND kind = $2 AND status = 'pending' ORDER BY run_after`, tenantID, string(kind))
	require.NoError(t, err)
	var out []time.Time
	for rows.Next() {
		var at time.Time
		require.NoError(t, rows.Scan(&at))
		out = append(out, at)
	}
	require.NoError(t, rows.Err())
	return out
}

// Taken over from the 1.47.0 notes: cancelling a held repeating job after a
// restore ended its chain for good (only the morning email carried on):
// the mailbox stopped syncing, the calendar stopped refreshing, a job-ads
// source was never checked again, a contact's email never re-checked.
// Cancelled, each skips that run and carries on at its next normal time,
// with one next run.
func TestCancellingAHeldRepeatingJobKeepsItsChainGoing(t *testing.T) {
	t.Parallel()

	t.Run("a job-ads source is checked again a day later", func(t *testing.T) {
		t.Parallel()
		a := newTestApp(t)
		tenant := testutil.Current.NewTenant(t, api.RoleOwner)
		owner := tenant.Users[api.RoleOwner].Token
		src := a.leadSource(owner, map[string]any{"kind": "careers_page", "identifier": "https://careers.contoh.example/jobs"})
		holdAndCancel(t, a, tenant.ID, owner, api.JobKindLeadSourceCheck)
		runs := waitingRuns(t, tenant.ID, api.JobKindLeadSourceCheck)
		require.Len(t, runs, 1, "one next check")
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), runs[0], 5*time.Minute)
		next, err := a.sourceByID(owner, src.Id).NextCheckAt.Get()
		require.NoError(t, err)
		assert.WithinDuration(t, runs[0], next, time.Second, "the source shows its next check")
	})

	t.Run("a contact's email is checked again next month, the last answer kept", func(t *testing.T) {
		t.Parallel()
		a := newTestApp(t)
		tenant := testutil.Current.NewTenant(t, api.RoleOwner)
		owner := tenant.Users[api.RoleOwner].Token
		company := a.createCompany(owner, map[string]any{"name": "Maybank"})
		contact := a.createContact(owner, map[string]any{"companyId": company.Id, "firstName": "Aisyah", "email": "aisyah@maybank.com.my"})
		ctx := context.Background()
		// Last month's answer, and this month's check again waiting.
		require.Equal(t, http.StatusAccepted, a.request(http.MethodPost, "/v1/contacts/"+contact.Id+"/mailbox-check", owner, nil).Status)
		_, err := testutil.Current.Admin.Exec(ctx, `UPDATE contact_mailbox_check SET status = 'accepted', checked_at = now() - interval '30 days'
			WHERE contact_id = $1`, contact.Id)
		require.NoError(t, err)
		_, err = testutil.Current.Admin.Exec(ctx, `UPDATE job SET payload = payload || '{"refresh": true}'
			WHERE tenant_id = $1 AND kind = 'contact.check_mailbox' AND status = 'pending'`, tenant.ID)
		require.NoError(t, err)
		holdAndCancel(t, a, tenant.ID, owner, api.JobKindContactCheckMailbox)
		runs := waitingRuns(t, tenant.ID, api.JobKindContactCheckMailbox)
		require.Len(t, runs, 1, "one next check")
		assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), runs[0], 5*time.Minute)
		var status string
		require.NoError(t, testutil.Current.Admin.QueryRow(ctx, `SELECT status FROM contact_mailbox_check WHERE contact_id = $1`, contact.Id).Scan(&status))
		assert.Equal(t, "accepted", status, "last month's answer stands")
	})

	t.Run("a mailbox carries on syncing", func(t *testing.T) {
		t.Parallel()
		a := newTestApp(t)
		tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
		recruiter, owner := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
		setupOutlookMail(t, a, recruiter)
		require.Equal(t, http.StatusNoContent, a.request(http.MethodPost, "/v1/outlook/mail", recruiter, nil).Status)
		runCoreJobsUntilIdle(t, a, tenant.ID) // the first sync runs and queues the next
		holdAndCancel(t, a, tenant.ID, owner, api.JobKindEmailOutlookSync)
		runs := waitingRuns(t, tenant.ID, api.JobKindEmailOutlookSync)
		require.Len(t, runs, 1, "one next sync")
		assert.WithinDuration(t, time.Now().Add(time.Minute), runs[0], 30*time.Second)
	})

	t.Run("a calendar carries on refreshing", func(t *testing.T) {
		t.Parallel()
		a := newTestApp(t)
		tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
		recruiter, owner := tenant.Users[api.RoleRecruiter].Token, tenant.Users[api.RoleOwner].Token
		setupOutlook(t, a)
		connectOutlook(t, a, recruiter)
		runCoreJobsUntilIdle(t, a, tenant.ID) // the first refresh runs and queues the next
		holdAndCancel(t, a, tenant.ID, owner, api.JobKindCalendarRefresh)
		runs := waitingRuns(t, tenant.ID, api.JobKindCalendarRefresh)
		require.Len(t, runs, 1, "one next refresh")
		assert.WithinDuration(t, time.Now().Add(5*time.Minute), runs[0], 30*time.Second)
	})
}
