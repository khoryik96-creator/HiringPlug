package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// An Outlook connection that is late, failing or needs reconnecting never
// blocks the candidate's booking page, and its host is emailed the invitation
// (Outlook cannot deliver it); the last known busy times still count.
func TestCalendarABrokenOutlookNeverBlocksBookingAndTheHostIsEmailed(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	host := tenant.Users[api.RoleRecruiter]
	setupOutlook(t, a)
	connectOutlook(t, a, host.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	link := openBookingLink(t, a, host.Token, tenant.ID)
	_, err := testutil.Current.Admin.Exec(t.Context(), `UPDATE calendar_connection SET status='reconnect' WHERE tenant_id=$1`, tenant.ID)
	require.NoError(t, err)
	_, err = testutil.Current.Admin.Exec(t.Context(), `UPDATE outlook_availability SET status='reconnect' WHERE tenant_id=$1`, tenant.ID)
	require.NoError(t, err)

	page := a.publicBooking(link) // used to answer 503 until the host reconnected
	require.NotEmpty(t, page.Slots)
	res := a.request(http.MethodPost, "/v1/public/bookings/"+link, "", map[string]any{"startsAt": page.Slots[0]})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	assert.NotEmpty(t, mailServer.To("recruiter."+tenant.ID+"@test.local"), "the host is emailed: their Outlook cannot take it")
}

// A clash with another interview names it (and can be scheduled over), with
// or without Outlook: the interview's own calendar hold is not reported as a
// nameless "calendar appointment".
func TestCalendarAnInterviewClashNamesTheOtherInterview(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	start := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Minute)
	first := scheduleInterview(t, a, user.Token, "Nadia", start, []string{user.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID) // its calendar hold is made
	application := applicationFor(t, a, user.Token, "Amir")
	body := map[string]any{"title": "Second interview", "startsAt": start.Add(15 * time.Minute), "durationMinutes": 30, "mode": "phone", "interviewerIds": []string{user.ID}}
	res := a.request(http.MethodPost, "/v1/applications/"+application+"/interviews", user.Token, body)
	require.Equal(t, 409, res.Status, "%s", res.Body)
	e := res.Error().Error
	require.NotNil(t, e.Details)
	clashes, _ := (*e.Details)["clashes"].([]any)
	require.Len(t, clashes, 1, "the clash is named, so the form offers Schedule anyway")
	assert.Equal(t, first.Id, clashes[0].(map[string]any)["interviewId"])
	assert.Empty(t, (*e.Details)["calendarBusy"], "its own hold is not a second, nameless clash")
}

// An interview deleted with its candidate leaves no hold behind: the next
// sync takes it out of Outlook and frees the time.
func TestCalendarADeletedInterviewLeavesOutlookAndFreesTheTime(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	iv := scheduleInterview(t, a, user.Token, "Nadia", time.Now().UTC().Add(48*time.Hour).Truncate(time.Minute), []string{user.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	require.Len(t, f.events, 1)
	f.mu.Unlock()
	_, err := testutil.Current.Admin.Exec(t.Context(), `DELETE FROM interview WHERE id=$1`, iv.Id) // as deleting its candidate does
	require.NoError(t, err)
	refreshOutlook(t, a, user.Token, tenant.ID)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Empty(t, f.events, "the Outlook copy is deleted")
	f.mu.Unlock()
	var busy int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT count(*) FROM outlook_busy WHERE tenant_id=$1 AND source_id=$2`, tenant.ID, iv.Id).Scan(&busy))
	assert.Zero(t, busy, "and the time is free again")
}

// Disconnecting takes HyPlug's interviews out of Outlook (nothing would
// update them) and leaves the person's own appointments where they are.
func TestCalendarDisconnectingTakesInterviewsOutOfOutlook(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	scheduleInterview(t, a, user.Token, "Nadia", start, []string{user.ID})
	own := createCalendarEvent(t, a, user.Token, map[string]any{"title": "Dentist", "notes": "", "startsAt": start.Add(3 * time.Hour), "endsAt": start.Add(4 * time.Hour)})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	require.Len(t, f.events, 2)
	f.mu.Unlock()
	res := a.request(http.MethodDelete, "/v1/calendar/connection", user.Token, nil)
	require.Equal(t, 204, res.Status, "%s", res.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.events, 1, "the interview is gone from Outlook")
	for _, e := range f.events {
		assert.Equal(t, "Dentist", e["subject"], "the person's own appointment stays")
	}
	_ = own
}

// Connecting another Microsoft account moves the interviews: they leave the
// old account instead of staying there, never updated again.
func TestCalendarSwitchingAccountsMovesInterviewsInsteadOfCopyingThem(t *testing.T) {
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
	f.account = "new-work-account"
	f.mu.Unlock()
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 2, f.creates, "made again in the new account")
	assert.Len(t, f.events, 1, "and no longer in the old one")
}

// A create Outlook stored but whose answer was lost is matched by its
// transaction ID when the next sync sees it: one event, no stuck push, and
// an appointment cancelled meanwhile is deleted from Outlook.
func TestCalendarACreateWhoseAnswerWasLostIsAdoptedNotDuplicated(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)

	f.mu.Lock()
	f.loseCreateAnswer = 1
	f.mu.Unlock()
	kept := createCalendarEvent(t, a, user.Token, map[string]any{"title": "Client lunch", "notes": "", "startsAt": start, "endsAt": start.Add(time.Hour)})
	runCoreJobsUntilIdle(t, a, tenant.ID) // the create is stored, its answer lost: the push waits to retry
	refreshOutlook(t, a, user.Token, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID) // and the retry, deduplicated by Outlook, finds the row already tied
	var rows, outlookRows int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT count(*), count(*) FILTER (WHERE source='outlook') FROM calendar_event WHERE tenant_id=$1`, tenant.ID).Scan(&rows, &outlookRows))
	assert.Equal(t, 1, rows, "one appointment, not an import beside it")
	assert.Zero(t, outlookRows)
	_, status := calendarEventState(t, kept.Id)
	assert.Equal(t, "synced", status)

	f.mu.Lock()
	f.loseCreateAnswer = 1
	f.mu.Unlock()
	gone := createCalendarEvent(t, a, user.Token, map[string]any{"title": "Cancelled call", "notes": "", "startsAt": start.Add(3 * time.Hour), "endsAt": start.Add(4 * time.Hour)})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, _ := calendarEventState(t, gone.Id)
	res := a.request(http.MethodPost, "/v1/calendar/events/"+gone.Id+"/cancel", user.Token, map[string]any{"version": version})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runPostponedCalendarJobs(t, a, tenant.ID)
	refreshOutlook(t, a, user.Token, tenant.ID)
	runPostponedCalendarJobs(t, a, tenant.ID)
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		assert.NotEqual(t, "Cancelled call", e["subject"], "the cancelled appointment is deleted from Outlook")
	}
	assert.Len(t, f.events, 1)
}

// Changing a Teams meeting's title leaves its body (with the join details)
// alone; notes of other appointments still go to Outlook.
func TestCalendarEditingATeamsMeetingKeepsItsJoinDetails(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	meeting := createCalendarEvent(t, a, user.Token, map[string]any{"title": "Teams call", "notes": "Agenda", "startsAt": start, "endsAt": start.Add(time.Hour), "teams": true})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, _ := calendarEventState(t, meeting.Id)
	res := a.request(http.MethodPut, "/v1/calendar/events/"+meeting.Id, user.Token, map[string]any{"version": version, "title": "Teams call (moved)", "notes": "Agenda", "startsAt": start, "endsAt": start.Add(time.Hour), "timeZone": "UTC", "attendees": []any{}, "allDay": false, "teams": true})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	require.NotNil(t, f.lastPatch)
	assert.Equal(t, "Teams call (moved)", f.lastPatch["subject"])
	assert.NotContains(t, f.lastPatch, "body", "the join details in its body stay")
	f.mu.Unlock()

	plain := createCalendarEvent(t, a, user.Token, map[string]any{"title": "Client lunch", "notes": "Old notes", "startsAt": start.Add(3 * time.Hour), "endsAt": start.Add(4 * time.Hour)})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	version, _ = calendarEventState(t, plain.Id)
	res = a.request(http.MethodPut, "/v1/calendar/events/"+plain.Id, user.Token, map[string]any{"version": version, "title": "Client lunch", "notes": "New notes", "startsAt": start.Add(3 * time.Hour), "endsAt": start.Add(4 * time.Hour), "timeZone": "UTC", "attendees": []any{}, "allDay": false, "teams": false})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Contains(t, f.lastPatch, "body", "changed notes of an ordinary appointment are sent")
}

// staleSources answers like a source job that read an older version of the
// interview: without its interviewer, at version 0.
type staleSources struct{ interview string }

func (s staleSources) Meeting(_ context.Context, _ pgx.Tx, kind, id string) (calendars.Meeting, error) {
	return calendars.Meeting{Kind: kind, ID: id, Version: 0, Users: []string{}, Start: time.Now().Add(48 * time.Hour), End: time.Now().Add(49 * time.Hour)}, nil
}
func (staleSources) MeetingsFor(context.Context, pgx.Tx, string, time.Time, time.Time) ([]calendars.Meeting, error) {
	return nil, nil
}
func (staleSources) Existing(_ context.Context, _ pgx.Tx, _ string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// A source job that read an older version of an interview never cancels the
// hold a newer version made.
func TestCalendarAnOlderInterviewVersionNeverCancelsANewerHold(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	iv := scheduleInterview(t, a, user.Token, "Nadia", time.Now().UTC().Add(48*time.Hour).Truncate(time.Minute), []string{user.ID})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	a.modules.Calendars.ConnectOutlookWith(calendars.Deps{Sources: staleSources{iv.Id}}, f.config)
	tx, err := testutil.Current.Admin.Begin(t.Context())
	require.NoError(t, err)
	_, err = jobqueue.Enqueue(t.Context(), tx, tenant.ID, api.JobKindCalendarSourceChanged, api.CalendarSourceJob{Kind: api.CalendarSourceJobKindInterview, SourceId: iv.Id})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(t.Context()))
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var status string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT status FROM calendar_event WHERE tenant_id=$1 AND source='interview' AND source_id=$2`, tenant.ID, iv.Id).Scan(&status))
	assert.Equal(t, "active", status)
}

// An appointment Outlook has, marked Free there, no longer blocks the time
// (the interval kept while it was on its way gives way to Outlook's).
func TestCalendarAnAppointmentMarkedFreeInOutlookDoesNotBlockTheTime(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	event := createCalendarEvent(t, a, user.Token, map[string]any{"title": "Focus time", "notes": "", "startsAt": start, "endsAt": start.Add(time.Hour)})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	refreshOutlook(t, a, user.Token, tenant.ID)
	busyFor := func() int {
		var n int
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT count(*) FROM outlook_busy WHERE tenant_id=$1 AND source_id=$2`, tenant.ID, event.Id).Scan(&n))
		return n
	}
	require.Equal(t, 1, busyFor(), "busy, as Outlook says")
	f.mu.Lock()
	for _, e := range f.events {
		e["showAs"] = "free"
	}
	f.mu.Unlock()
	refreshOutlook(t, a, user.Token, tenant.ID)
	assert.Zero(t, busyFor(), "marked Free in Outlook: free")
}

func openBookingLink(t *testing.T, a *testApp, token, tenant string) string {
	t.Helper()
	var hours []map[string]any
	for day := 1; day <= 7; day++ {
		hours = append(hours, map[string]any{"day": day, "start": "08:00", "end": "18:00"})
	}
	res := a.request(http.MethodPut, "/v1/scheduling/me", token, map[string]any{"timeZone": "Asia/Kuala_Lumpur", "hours": hours, "bufferMinutes": 0, "minNoticeHours": 2, "maxDaysAhead": 3})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	candidate := a.createCandidate(token, map[string]any{"firstName": "Nadia", "email": "booking-" + tenant + "@example.test"})
	link := a.bookingLink(token, map[string]any{"candidateId": candidate.Id, "title": "Screening call", "durationMinutes": 30})
	return tokenInURL.FindStringSubmatch(link.Url.MustGet())[1]
}

func applicationFor(t *testing.T, a *testApp, token, name string) string {
	t.Helper()
	company := a.createCompany(token, map[string]any{"name": "Calendar agency " + name})
	job := a.createJob(token, map[string]any{"companyId": company.Id, "title": "Engineer"})
	candidate := a.createCandidate(token, map[string]any{"firstName": name})
	return a.addToJob(token, job.Id, candidate.Id).Id
}

func scheduleInterview(t *testing.T, a *testApp, token, name string, start time.Time, interviewers []string) api.Interview {
	t.Helper()
	res := a.request(http.MethodPost, "/v1/applications/"+applicationFor(t, a, token, name)+"/interviews", token, map[string]any{"title": "Interview", "startsAt": start, "durationMinutes": 60, "mode": "video", "interviewerIds": interviewers})
	require.Equal(t, 201, res.Status, "%s", res.Body)
	var iv api.Interview
	res.JSON(&iv)
	return iv
}

func createCalendarEvent(t *testing.T, a *testApp, token string, body map[string]any) api.CalendarEvent {
	t.Helper()
	full := map[string]any{"timeZone": "UTC", "attendees": []any{}, "allDay": false, "teams": false}
	for k, v := range body {
		full[k] = v
	}
	res := a.request(http.MethodPost, "/v1/calendar/events", token, full)
	require.Equal(t, 201, res.Status, "%s", res.Body)
	var e api.CalendarEvent
	res.JSON(&e)
	return e
}
