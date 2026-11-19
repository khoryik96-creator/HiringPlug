package app_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/app"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

type outlookSendTest struct {
	a                         *testApp
	provider                  *draftProvider
	tenant, user, owner, path string
	draft                     api.OutlookDraft
}

func startOutlookSendTest(t *testing.T, afterRequest func(*http.Request)) outlookSendTest {
	t.Helper()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	user := tenant.Users[api.RoleRecruiter].Token
	f := newDraftProvider()
	connectTestOutlook(t, a, user, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.serve(w, r)
		if afterRequest != nil {
			afterRequest(r)
		}
	}))
	require.Equal(t, 204, a.request("POST", "/v1/outlook/mail", user, nil).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	draft := createDraftForTest(t, a, user)
	return outlookSendTest{a, f, tenant.ID, user, tenant.Users[api.RoleOwner].Token, "/v1/outlook/drafts/" + draft.MessageId, draft}
}

func (r outlookSendTest) queue(t *testing.T, in map[string]any) api.OutlookScheduledSend {
	t.Helper()
	in["draftVersion"] = r.draft.Version
	res := r.a.request("PUT", r.path+"/send", r.user, in)
	require.Equal(t, 202, res.Status, "%s", res.Body)
	var row api.OutlookScheduledSend
	res.JSON(&row)
	return row
}

func (r outlookSendTest) list(t *testing.T) []api.OutlookScheduledSend {
	t.Helper()
	res := r.a.request("GET", "/v1/outlook/scheduled", r.user, nil)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	var page api.OutlookScheduleList
	res.JSON(&page)
	return page.Items
}

func (r outlookSendTest) due(t *testing.T, id string) {
	t.Helper()
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_send SET send_at=now()-interval '1 second' WHERE id=$1", id)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(t.Context(), "UPDATE job SET run_after=now() WHERE tenant_id=$1 AND kind='email.outlook_scheduled' AND status='pending'", r.tenant)
	require.NoError(t, err)
}

func (r outlookSendTest) sends() int {
	r.provider.mu.Lock()
	defer r.provider.mu.Unlock()
	return r.provider.attempts
}

func TestOutlookRescheduleEditsAndTimeTogether(t *testing.T) {
	t.Parallel()
	patched := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	r := startOutlookSendTest(t, func(req *http.Request) {
		if req.Method == http.MethodPatch && strings.HasPrefix(req.URL.Path, "/me/messages/") {
			close(patched)
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	})
	first := r.queue(t, map[string]any{"expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
	r.due(t, first.Id)

	later := time.Now().Add(25 * time.Hour).Truncate(time.Second)
	done := make(chan *response, 1)
	go func() {
		done <- r.a.request("PUT", r.path+"/send", r.user, map[string]any{
			"draftVersion": r.draft.Version, "expectedVersion": first.Version,
			"changes": map[string]any{"subject": "Tomorrow's message"}, "sendAt": later,
		})
	}()
	select {
	case <-patched:
	case response := <-done:
		t.Fatalf("Combined edit did not patch the provider draft: %d %s", response.Status, response.Body)
	case <-ctx.Done():
		t.Fatal("Outlook draft edit did not start")
	}
	w, err := app.NewWorker(r.a.deps, r.a.modules)
	require.NoError(t, err)
	w.Tenant = r.tenant
	ran, err := w.RunOnce(ctx)
	require.NoError(t, err)
	require.True(t, ran)
	require.Zero(t, r.sends(), "the old job cannot enter while the edit holds the mailbox lock")
	close(release)
	var res *response
	select {
	case res = <-done:
	case <-ctx.Done():
		t.Fatal("Reschedule did not finish")
	}
	require.Equal(t, 202, res.Status, "%s", res.Body)
	var updated api.OutlookScheduledSend
	res.JSON(&updated)
	require.Equal(t, first.Id, updated.Id)
	require.Equal(t, first.Version+1, updated.Version)
	require.Equal(t, "Tomorrow's message", updated.Subject)
	require.True(t, later.Equal(updated.SendAt))

	r.due(t, updated.Id)
	runCoreJobsUntilIdle(t, r.a, r.tenant)
	require.Equal(t, 1, r.sends())
	require.Equal(t, api.OutlookScheduledSendStatusSent, r.list(t)[0].Status)
}

func TestOutlookScheduleChecksUnlinkedRecipientsAndLaterConsent(t *testing.T) {
	t.Parallel()
	r := startOutlookSendTest(t, nil)
	c := r.a.createCandidate(r.owner, map[string]any{"firstName": "Nadia", "email": "nadia@example.test"})
	r.a.setRole(r.owner, "recruiter", map[string]any{
		"version": 0, "permissions": authz.PermissionsForRole(api.RoleRecruiter), "candidateScope": "own", "jobCandidates": false,
	}, http.StatusOK)
	require.Equal(t, 404, r.a.request("GET", "/v1/candidates/"+c.Id, r.user, nil).Status)
	r.a.setConsent(r.owner, c.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	for _, field := range []string{"to", "cc", "bcc"} {
		changes := map[string]any{"to": []string{"another@example.test"}}
		changes[field] = []string{"Nadia@EXAMPLE.TEST"}
		res := r.a.request("PUT", r.path+"/send", r.user, map[string]any{
			"draftVersion": r.draft.Version, "expectedVersion": 0, "changes": changes,
		})
		require.Equal(t, 409, res.Status, "%s %s", field, res.Body)
		require.Equal(t, api.ErrorCodeEMAILDONOTCONTACT, res.Error().Error.Code)
	}
	require.Empty(t, r.list(t))

	r.a.setConsent(r.owner, c.Id, map[string]any{"expectedVersion": 1, "status": "given", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	other := testutil.Current.NewTenant(t, api.RoleOwner).Users[api.RoleOwner].Token
	otherCandidate := r.a.createCandidate(other, map[string]any{"firstName": "Other", "email": "nadia@example.test"})
	r.a.setConsent(other, otherCandidate.Id, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	first := r.queue(t, map[string]any{"expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
	r.a.setConsent(r.owner, c.Id, map[string]any{"expectedVersion": 2, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	r.due(t, first.Id)
	runCoreJobsUntilIdle(t, r.a, r.tenant)
	require.Zero(t, r.sends())
	require.Equal(t, api.OutlookScheduledSendStatusNeedsReview, r.list(t)[0].Status)
}

func TestOutlookScheduledDraftOpensAfterIndexReset(t *testing.T) {
	t.Parallel()
	r := startOutlookSendTest(t, nil)
	send := r.queue(t, map[string]any{"expectedVersion": 0, "sendAt": time.Now().Add(time.Hour)})
	_, err := testutil.Current.Admin.Exec(t.Context(), "DELETE FROM email_outlook_message WHERE id=$1", r.draft.MessageId)
	require.NoError(t, err)

	for range 2 {
		got := r.a.request("GET", r.path, r.user, nil)
		require.Equal(t, 200, got.Status, "%s", got.Body)
		var fresh api.OutlookDraft
		got.JSON(&fresh)
		require.NotEqual(t, r.draft.MessageId, fresh.MessageId)
		require.Equal(t, send.Id, fresh.Schedule.MustGet().Id)
	}
	updated := r.queue(t, map[string]any{"expectedVersion": send.Version, "sendAt": time.Now().Add(2 * time.Hour)})
	require.Equal(t, send.Id, updated.Id)
	require.NotEqual(t, send.MessageId, updated.MessageId)
}

func TestOutlookDeadReceiptJobRequiresCheckingOutlook(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var checking atomic.Bool
	r := startOutlookSendTest(t, func(req *http.Request) {
		if checking.Load() && req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/me/messages/") {
			cancel()
		}
	})
	send := r.queue(t, map[string]any{"expectedVersion": 0})
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE email_outlook_send SET status='confirming',attempted_at=now()-interval '30 minutes' WHERE id=$1", send.Id)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(t.Context(), "UPDATE job SET attempts=max_attempts-1 WHERE tenant_id=$1 AND kind='email.outlook_scheduled'", r.tenant)
	require.NoError(t, err)
	w, err := app.NewWorker(r.a.deps, r.a.modules)
	require.NoError(t, err)
	w.Tenant = r.tenant
	checking.Store(true)
	ran, err := w.RunOnce(ctx)
	require.NoError(t, err)
	require.True(t, ran)

	var status string
	err = testutil.Current.Admin.QueryRow(t.Context(), "SELECT status FROM job WHERE tenant_id=$1 AND kind='email.outlook_scheduled'", r.tenant).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "dead", status)
	require.Equal(t, api.OutlookScheduledSendStatusUncertain, r.list(t)[0].Status)
	require.Zero(t, r.sends(), "an uncertain message is never resubmitted during recovery")
}
