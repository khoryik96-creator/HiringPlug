package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestCalendarOAuthIsPrivateAndReconnectKeepsRemoteIdentity(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	token := tenant.Users[api.RoleRecruiter].Token
	f := setupOutlook(t, a)
	ticket := outlookTicket(t, a, token)
	assert.Equal(t, 400, a.request("POST", "/v1/calendar/complete", tenant.Users[api.RoleOwner].Token, map[string]any{"ticket": ticket}).Status)
	r := a.request("POST", "/v1/calendar/complete", token, map[string]any{"ticket": ticket})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	assert.NotContains(t, string(r.Body), "synthetic-access")
	assert.Equal(t, 400, a.request("POST", "/v1/calendar/complete", token, map[string]any{"ticket": ticket}).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	input := calendarInput(start)
	input["attendees"] = []any{map[string]any{"email": "invitee@another-domain.test"}}
	r = a.request("POST", "/v1/calendar/events", token, input)
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var event api.CalendarEvent
	r.JSON(&event)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	_, status := calendarEventState(t, event.Id)
	assert.Equal(t, "synced", status)
	f.mu.Lock()
	assert.Equal(t, 1, f.creates)
	assert.Equal(t, 1, f.refreshes, "rotated token survives later queue transactions")
	f.mu.Unlock()
	require.Equal(t, 204, a.request("DELETE", "/v1/calendar/connection", token, nil).Status)
	connectOutlook(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Equal(t, 1, f.creates, "reconnecting the same account must not create a duplicate")
	assert.Equal(t, 0, f.patches, "reconnecting must not resend invitations for unchanged appointments")
	f.mu.Unlock()
	r = a.request("POST", "/v1/calendar/events/"+event.Id+"/cancel", token, map[string]any{"version": event.Version})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Empty(t, f.events)
	f.mu.Unlock()
	var encrypted string
	require.NoError(t, testutil.Current.Admin.QueryRow(context.Background(), "SELECT sealed_tokens FROM calendar_connection WHERE tenant_id=$1 AND user_id=$2", tenant.ID, tenant.Users[api.RoleRecruiter].ID).Scan(&encrypted))
	assert.NotContains(t, encrypted, "synthetic")
}
func TestCalendarTransientPushRetriesAndConflictsNeedExplicitResolution(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := setupOutlook(t, a)
	connectOutlook(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	f.writeStatus = 503
	f.mu.Unlock()
	input := calendarInput(time.Now().UTC().Add(48 * time.Hour))
	r := a.request("POST", "/v1/calendar/events", token, input)
	require.Equal(t, 201, r.Status)
	var event api.CalendarEvent
	r.JSON(&event)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	_, status := calendarEventState(t, event.Id)
	assert.Equal(t, "pending", status, "a temporary provider error must remain retryable")
	f.mu.Lock()
	f.writeStatus = 0
	f.mu.Unlock()
	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE job SET run_after=now() WHERE tenant_id=$1 AND kind='calendar.push' AND status='pending'", tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, status := calendarEventState(t, event.Id)
	require.Equal(t, "synced", status)
	input["version"] = version
	input["title"] = "Changed in HiringPlug"
	r = a.request("PUT", "/v1/calendar/events/"+event.Id, token, input)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	f.mu.Lock()
	f.writeStatus = 412
	f.mu.Unlock()
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, status = calendarEventState(t, event.Id)
	assert.Equal(t, "conflict", status, "pending local edits must still detect a provider conflict")
	f.mu.Lock()
	f.writeStatus = 0
	f.mu.Unlock()
	r = a.request("POST", "/v1/calendar/events/"+event.Id+"/retry", token, map[string]any{"version": version})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, status = calendarEventState(t, event.Id)
	assert.Equal(t, "synced", status)
	// Deleting in Outlook between a local edit and its push needs explicit
	// recreation, with a new creation identity and no automatic overwrite.
	f.mu.Lock()
	f.events = map[string]map[string]any{}
	f.mu.Unlock()
	input["version"] = version
	input["title"] = "Recreated after review"
	r = a.request("PUT", "/v1/calendar/events/"+event.Id, token, input)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, status = calendarEventState(t, event.Id)
	require.Equal(t, "conflict", status)
	r = a.request("POST", "/v1/calendar/events/"+event.Id+"/retry", token, map[string]any{"version": version})
	require.Equal(t, 200, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	_, status = calendarEventState(t, event.Id)
	assert.Equal(t, "synced", status)
	var generation int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT remote_generation FROM calendar_event WHERE id=$1", event.Id).Scan(&generation))
	assert.Equal(t, 1, generation)
	f.mu.Lock()
	assert.Len(t, f.events, 1)
	f.mu.Unlock()
}
func TestCalendarPartialPagesKeepBusyTimesAndCanBeScheduledOver(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	user := tenant.Users[api.RoleRecruiter].ID
	f := setupOutlook(t, a)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	f.events["external"] = map[string]any{"id": "external", "subject": "Private dentist appointment", "start": map[string]string{"dateTime": start.Format(time.RFC3339), "timeZone": "UTC"}, "end": map[string]string{"dateTime": start.Add(time.Hour).Format(time.RFC3339), "timeZone": "UTC"}, "showAs": "busy", "changeKey": "v1"}
	connectOutlook(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var count int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT count(*) FROM outlook_busy WHERE tenant_id=$1", tenant.ID).Scan(&count))
	require.Equal(t, 1, count)
	f.mu.Lock()
	f.brokenPages = true
	f.mu.Unlock()
	refreshOutlook(t, a, token, tenant.ID)
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT count(*) FROM outlook_busy WHERE tenant_id=$1", tenant.ID).Scan(&count))
	assert.Equal(t, 1, count, "incomplete snapshots must not clear busy times")
	company := a.createCompany(token, map[string]any{"name": "Synthetic agency"})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Engineer"})
	candidate := a.createCandidate(token, map[string]any{"firstName": "Synthetic"})
	application := a.addToJob(token, job.Id, candidate.Id)
	body := map[string]any{"title": "Interview", "startsAt": start, "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{user}}
	r := a.request("POST", "/v1/applications/"+application.Id+"/interviews", token, body)
	// A sync that failed keeps the last busy times and says so; it never
	// blocks scheduling outright (it used to answer 503 with no way past it).
	require.Equal(t, 409, r.Status, "%s", r.Body)
	e := r.Error().Error
	assert.Equal(t, api.ErrorCodeINTERVIEWCLASH, e.Code)
	require.NotNil(t, e.Details)
	assert.Len(t, (*e.Details)["calendarBusy"], 1, "the dentist appointment, by its owner's name only")
	assert.Len(t, (*e.Details)["calendarStale"], 1, "and the failed sync")
	assert.NotContains(t, string(r.Body), "dentist")
	body["ignoreClashes"] = true
	r = a.request("POST", "/v1/applications/"+application.Id+"/interviews", token, body)
	assert.Equal(t, 201, r.Status, "scheduled anyway: %s", r.Body)
}

// Review of 1.47.0: an event's push held after a restore and cancelled
// stayed "pending" (being sent) for ever. It fails, saying why, and can be
// sent again.
func TestACalendarPushCancelledAfterARestoreSaysSo(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner)
	token := tenant.Users[api.RoleRecruiter].Token
	setupOutlook(t, a)
	connectOutlook(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	r := a.request("POST", "/v1/calendar/events", token, calendarInput(time.Now().UTC().Add(72*time.Hour).Truncate(time.Minute)))
	require.Equal(t, 201, r.Status, "%s", r.Body)
	var event api.CalendarEvent
	r.JSON(&event)
	var jobID string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `UPDATE job SET status = 'held', held_at = now()
		WHERE tenant_id = $1 AND kind = 'calendar.push' AND status = 'pending' RETURNING id`, tenant.ID).Scan(&jobID))
	r = a.request("POST", "/v1/settings/system/jobs/"+jobID+"/cancel", tenant.Users[api.RoleOwner].Token, nil)
	require.Equal(t, 200, r.Status, "%s", r.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	_, status := calendarEventState(t, event.Id)
	assert.Equal(t, "failed", status, "not being sent for ever")
	var why string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT sync_error FROM calendar_event WHERE id = $1`, event.Id).Scan(&why))
	assert.Contains(t, why, "cancelled after the server was restored")
}
