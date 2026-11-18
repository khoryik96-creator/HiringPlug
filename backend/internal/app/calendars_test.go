package app_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func calendarInput(start time.Time) map[string]any {
	return map[string]any{"title": "Interview preparation", "notes": "Private notes", "location": "Office", "startsAt": start, "endsAt": start.Add(time.Hour), "timeZone": "Asia/Kuala_Lumpur", "allDay": false, "teams": false, "attendees": []any{}}
}
func calendarRange(start time.Time) string {
	return "/v1/calendar/events?from=" + url.QueryEscape(start.Add(-time.Hour).Format(time.RFC3339)) + "&to=" + url.QueryEscape(start.Add(24*time.Hour).Format(time.RFC3339))
}
func TestCalendarPrivateCRUDAndVersions(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleOwner, api.RoleViewer)
	user := tenant.Users[api.RoleRecruiter].Token
	start := time.Now().UTC().Truncate(time.Hour).Add(24 * time.Hour)
	body := calendarInput(start)
	res := a.request(http.MethodPost, "/v1/calendar/events", user, body)
	require.Equal(t, 201, res.Status, "%s", res.Body)
	var event api.CalendarEvent
	res.JSON(&event)
	assert.Equal(t, api.CalendarEventSyncStatusLocal, event.SyncStatus)
	path := "/v1/calendar/events/" + event.Id
	body["version"] = event.Version
	body["title"] = "Updated"
	res = a.request(http.MethodPut, path, user, body)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	res.JSON(&event)
	assert.Equal(t, 409, a.request(http.MethodPut, path, user, body).Status, "stale edits cannot overwrite appointments")
	var list api.CalendarEventList
	a.request(http.MethodGet, calendarRange(start), tenant.Users[api.RoleOwner].Token, nil).JSON(&list)
	assert.Empty(t, list.Items, "even the agency owner cannot read another user's calendar")
	assert.Equal(t, 404, a.request(http.MethodPut, path, tenant.Users[api.RoleOwner].Token, body).Status)
	other := testutil.Current.NewTenant(t, api.RoleRecruiter)
	a.request(http.MethodGet, calendarRange(start), other.Users[api.RoleRecruiter].Token, nil).JSON(&list)
	assert.Empty(t, list.Items)
	assert.Equal(t, 403, a.request(http.MethodGet, calendarRange(start), tenant.Users[api.RoleViewer].Token, nil).Status)
	body["attendees"] = []any{map[string]any{"email": "person@example.test"}}
	body["version"] = event.Version
	assert.Equal(t, 409, a.request(http.MethodPut, path, user, body).Status, "inviting people requires Outlook")
	res = a.request(http.MethodPost, path+"/cancel", user, map[string]any{"version": event.Version})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	a.request(http.MethodGet, calendarRange(start), user, nil).JSON(&list)
	assert.Empty(t, list.Items)
}
func TestCalendarValidationAndDisconnectedStatus(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	res := a.request(http.MethodGet, "/v1/calendar/connection", token, nil)
	require.Equal(t, 200, res.Status)
	var status api.OutlookConnection
	res.JSON(&status)
	assert.False(t, status.Ready)
	assert.False(t, status.Connected)
	assert.Equal(t, 400, a.request(http.MethodPost, "/v1/calendar/authorize", token, nil).Status)
	in := calendarInput(time.Now().UTC())
	in["endsAt"] = in["startsAt"]
	assert.Equal(t, 400, a.request(http.MethodPost, "/v1/calendar/events", token, in).Status)
	assert.Equal(t, 400, a.request(http.MethodGet, "/v1/calendar/events?from=2026-01-01T00:00:00Z&to=2027-01-01T00:00:00Z", token, nil).Status)
}
