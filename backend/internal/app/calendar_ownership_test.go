package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// An interview on a candidate the scheduler may not see still makes its
// interviewer busy: named only as a calendar appointment, never treated as
// gone (which double-booked them).
func TestCalendarAnInterviewTheSchedulerCannotSeeStillMakesItsInterviewerBusy(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleHiringManager)
	owner := tenant.Users[api.RoleOwner]
	recruiter := tenant.Users[api.RoleRecruiter]
	manager := tenant.Users[api.RoleHiringManager]
	a.setRole(owner.Token, "recruiter", map[string]any{"version": 0, "permissions": authz.PermissionsForRole(api.RoleRecruiter), "candidateScope": "own", "jobCandidates": false}, http.StatusOK)
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
	scheduleInterview(t, a, owner.Token, "Nadia", start, []string{manager.ID}) // the owner's candidate
	runCoreJobsUntilIdle(t, a, tenant.ID)

	application := applicationFor(t, a, recruiter.Token, "Amir")
	res := a.request(http.MethodPost, "/v1/applications/"+application+"/interviews", recruiter.Token, map[string]any{"title": "Second interview", "startsAt": start, "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{manager.ID}})
	require.Equal(t, 409, res.Status, "the hiring manager is busy then: %s", res.Body)
	e := res.Error().Error
	require.NotNil(t, e.Details)
	assert.Len(t, (*e.Details)["calendarBusy"], 1, "as a calendar appointment")
	assert.NotContains(t, string(res.Body), "Nadia", "nothing of the interview the recruiter may not see")
}

// Connecting another account that fails (its calendar belongs to a colleague)
// leaves the old Outlook as it was: its interviews are not taken out first.
func TestCalendarAFailedAccountSwitchLeavesTheOldCalendarAlone(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager)
	user := tenant.Users[api.RoleRecruiter]
	colleague := tenant.Users[api.RoleHiringManager]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	scheduleInterview(t, a, user.Token, "Nadia", time.Now().UTC().Add(48*time.Hour).Truncate(time.Minute), []string{user.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	f.account = "shared-account"
	f.mu.Unlock()
	connectOutlook(t, a, colleague.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	ticket := outlookTicket(t, a, user.Token)
	res := a.request(http.MethodPost, "/v1/calendar/complete", user.Token, map[string]any{"ticket": ticket})
	require.NotEqual(t, 200, res.Status, "that calendar is the colleague's")
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := 0
	for _, e := range f.events {
		if accountOf(e) == "synthetic-account" {
			kept++
		}
	}
	assert.Equal(t, 1, kept, "the interview is still in the old Outlook")
}

// Disconnecting while Outlook refuses the deletes keeps the interviews' links,
// so reconnecting the account updates them instead of making copies; those
// deleted come back with a new transaction ID.
func TestCalendarDisconnectForgetsOnlyWhatOutlookDeleted(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	scheduleInterview(t, a, user.Token, "Nadia", time.Now().UTC().Add(48*time.Hour).Truncate(time.Minute), []string{user.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID)

	f.mu.Lock()
	f.deleteStatus = 503
	f.mu.Unlock()
	require.Equal(t, 204, a.request(http.MethodDelete, "/v1/calendar/connection", user.Token, nil).Status)
	f.mu.Lock()
	f.deleteStatus = 0
	f.mu.Unlock()
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Len(t, f.events, 1, "still one: the kept link updates it")
	assert.Equal(t, 1, f.creates)
	var first string
	for _, e := range f.events {
		first, _ = e["transactionId"].(string)
	}
	f.mu.Unlock()

	require.Equal(t, 204, a.request(http.MethodDelete, "/v1/calendar/connection", user.Token, nil).Status)
	f.mu.Lock()
	require.Empty(t, f.events, "deleted this time")
	f.mu.Unlock()
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.events, 1, "made again")
	for _, e := range f.events {
		assert.NotEqual(t, first, e["transactionId"], "with a new transaction ID (Outlook would deduplicate the deleted one's)")
	}
}

// A stale hold is omitted from reads without changing the calendar or queuing work.
func TestCalendarReadsHideStaleHoldsWithoutChangingThem(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	iv := scheduleInterview(t, a, user.Token, "Nadia", start, []string{user.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	_, err := testutil.Current.Admin.Exec(t.Context(), `DELETE FROM interview WHERE id=$1`, iv.Id)
	require.NoError(t, err)
	window := "?from=" + start.Add(-time.Hour).Format(time.RFC3339) + "&to=" + start.Add(2*time.Hour).Format(time.RFC3339)
	for range 3 {
		res := a.request(http.MethodGet, "/v1/calendar/events"+window, user.Token, nil)
		require.Equal(t, 200, res.Status, "%s", res.Body)
		assert.NotContains(t, string(res.Body), iv.Id)
	}
	var status string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT status FROM calendar_event WHERE tenant_id=$1 AND source_id=$2`, tenant.ID, iv.Id).Scan(&status))
	assert.Equal(t, "active", status, "calendar GET leaves reconciliation to the worker")
	var queued int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT count(*) FROM job WHERE tenant_id=$1 AND kind='calendar.source_changed' AND status='pending'`, tenant.ID).Scan(&queued))
	assert.Zero(t, queued, "no job per view")
}

// A calendar connected a moment ago (its first sync on its way) is not
// "stopped syncing": scheduling with its owner is not held up.
func TestCalendarAJustConnectedCalendarIsNotReportedAsUnsynced(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	setupOutlook(t, a)
	connectOutlook(t, a, user.Token) // its first sync not run yet
	application := applicationFor(t, a, user.Token, "Nadia")
	res := a.request(http.MethodPost, "/v1/applications/"+application+"/interviews", user.Token, map[string]any{"title": "Interview", "startsAt": time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute), "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{user.ID}})
	assert.Equal(t, 201, res.Status, "%s", res.Body)
}

// An interviewer taken off a panel is free again as soon as HyPlug says
// so, even while Outlook has not yet deleted the copy.
func TestCalendarARemovedPanelMemberIsFreeBeforeOutlookDeletesTheCopy(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager)
	user := tenant.Users[api.RoleRecruiter]
	manager := tenant.Users[api.RoleHiringManager]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	iv := scheduleInterview(t, a, user.Token, "Nadia", start, []string{user.ID, manager.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	f.deleteStatus = 503 // the delete waits to be retried
	f.mu.Unlock()
	res := a.request(http.MethodPatch, "/v1/interviews/"+iv.Id, user.Token, map[string]any{"version": iv.Version, "interviewerIds": []string{manager.ID}})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	refreshOutlook(t, a, user.Token, tenant.ID) // Outlook still has the copy
	var busy int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT count(*) FROM outlook_busy WHERE tenant_id=$1 AND user_id=$2 AND source_id=$3`, tenant.ID, user.ID, iv.Id).Scan(&busy))
	assert.Zero(t, busy)
}
