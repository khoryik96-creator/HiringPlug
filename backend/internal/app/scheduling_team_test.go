package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// setHours saves every day 08:00-18:00 in Kuala Lumpur, bookable 2 hours on.
func (a *testApp) setHours(token string, extra map[string]any) {
	a.t.Helper()
	var hours []map[string]any
	for d := 1; d <= 7; d++ {
		hours = append(hours, map[string]any{"day": d, "start": "08:00", "end": "18:00"})
	}
	body := map[string]any{"timeZone": "Asia/Kuala_Lumpur", "hours": hours, "bufferMinutes": 0, "minNoticeHours": 2, "maxDaysAhead": 5}
	for k, v := range extra {
		body[k] = v
	}
	res := a.request(http.MethodPut, "/v1/scheduling/me", token, body)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
}

func TestBookingInvitationsGoFromTheRecruitersOwnMailbox(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	mailbox := "aina." + tenant.ID + "@agency.test"
	res := a.request(http.MethodPut, "/v1/mailbox", recruiter, smtpMailboxBody(t, mailbox, mailServer.Addr))
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	a.setHours(recruiter, nil)
	candidate := "siti." + tenant.ID + "@mail.test"
	cand := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "email": candidate})
	link := a.bookingLink(recruiter, map[string]any{"candidateId": cand.Id, "title": "Call", "durationMinutes": 30})
	token := tokenInURL.FindStringSubmatch(link.Url.MustGet())[1]

	res = a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{"startsAt": a.publicBooking(token).Slots[0]})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	msgs := mailServer.To(candidate)
	require.Len(t, msgs, 1)
	assert.Equal(t, mailbox, msgs[0].From, "sent through the recruiter's connected mailbox")
	assert.Contains(t, msgs[0].Data, "Test recruiter", "in their name")
	assert.Contains(t, invitation(t, msgs[0].Data), "METHOD:REQUEST")
}

func TestOnlyTheOwnerSeesEveryonesBookingSetUpButNeverTheirCalendarAddress(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleOwner, api.RoleRecruiter, api.RoleAdmin, api.RoleViewer)
	owner, recruiter := tenant.Users[api.RoleOwner].Token, tenant.Users[api.RoleRecruiter].Token
	cal := calendarServer(t, time.Now().AddDate(0, 0, 1))
	a.setHours(recruiter, map[string]any{"calendarUrl": cal.URL + "/calendar/ical/aina%40gmail.com/private-secretcode/basic.ics"})
	runCoreJobsUntilIdle(t, a, tenant.ID)
	cand := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "email": "siti." + tenant.ID + "@mail.test"})
	a.bookingLink(recruiter, map[string]any{"candidateId": cand.Id, "title": "Call", "durationMinutes": 30})

	// Recruiters and admins see their own set-up only.
	for _, token := range []string{recruiter, tenant.Users[api.RoleAdmin].Token} {
		assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/scheduling/team", token, nil).Status)
	}
	res := a.request(http.MethodGet, "/v1/scheduling/team", owner, nil)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.NotContains(t, string(res.Body), "secretcode", "a calendar address stays private to its owner")
	var team struct{ Items []api.TeamSchedule }
	res.JSON(&team)
	byRole := map[string]api.TeamSchedule{}
	for _, s := range team.Items {
		byRole[strings.ToLower(strings.TrimPrefix(s.Name, "Test "))] = s
	}
	require.Contains(t, byRole, "recruiter")
	r := byRole["recruiter"]
	assert.True(t, r.Set)
	assert.Equal(t, "Asia/Kuala_Lumpur", r.TimeZone)
	assert.Equal(t, api.CalendarFeedStatusOk, r.Calendar.MustGet().Status)
	assert.Equal(t, "127.0.0.1", r.Calendar.MustGet().Host)
	assert.Equal(t, 1, r.OpenLinks)
	assert.Zero(t, r.Upcoming)
	assert.True(t, r.NextAt.IsNull())
	assert.False(t, byRole["owner"].Set, "not set up yet")
	assert.True(t, byRole["owner"].Calendar.IsNull())
	assert.NotContains(t, byRole, "viewer", "only those who may take bookings")
}
