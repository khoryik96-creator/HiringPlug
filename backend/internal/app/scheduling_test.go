package app_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// calendarServer serves an .ics calendar with one busy hour, as Google
// Calendar's secret address does.
func calendarServer(t *testing.T, busyStart time.Time) *httptest.Server {
	t.Helper()
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Google Inc//Google Calendar//EN\r\n" +
		"BEGIN:VEVENT\r\nUID:busy@google.com\r\nDTSTART:" + busyStart.UTC().Format("20060102T150405Z") +
		"\r\nDTEND:" + busyStart.Add(time.Hour).UTC().Format("20060102T150405Z") + "\r\nSUMMARY:Private meeting\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (a *testApp) publicBooking(token string) api.PublicBooking {
	a.t.Helper()
	res := a.request(http.MethodGet, "/v1/public/bookings/"+token, "", nil)
	require.Equal(a.t, http.StatusOK, res.Status, "%s", res.Body)
	var out api.PublicBooking
	res.JSON(&out)
	return out
}

func (a *testApp) bookingLink(token string, body map[string]any) api.BookingLink {
	a.t.Helper()
	res := a.request(http.MethodPost, "/v1/booking-links", token, body)
	require.Equal(a.t, http.StatusCreated, res.Status, "%s", res.Body)
	var out api.BookingLink
	res.JSON(&out)
	return out
}

var tokenInURL = regexp.MustCompile(`/book/([A-Za-z0-9_-]{43})$`)

// invitation is the decoded text/calendar part of a sent message.
func invitation(t *testing.T, data string) string {
	t.Helper()
	i := strings.Index(data, "Content-Type: text/calendar")
	require.GreaterOrEqual(t, i, 0, "no invitation in the message")
	rest := data[i:]
	body := rest[strings.Index(rest, "\r\n\r\n")+4:]
	body = body[:strings.Index(body, "\r\n--")]
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body, "\r\n", ""))
	require.NoError(t, err)
	return string(decoded)
}

func TestBookingLinksBlockBothCalendars(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleViewer)
	recruiter := tenant.Users[api.RoleRecruiter].Token
	hostEmail := "recruiter." + tenant.ID + "@test.local"
	cand := a.createCandidate(recruiter, map[string]any{"firstName": "Siti", "lastName": "Aisyah", "email": "siti." + tenant.ID + "@mail.test"})

	res := a.request(http.MethodPost, "/v1/booking-links", recruiter, map[string]any{"candidateId": cand.Id, "title": "Call", "durationMinutes": 30})
	require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
	assert.Equal(t, api.ErrorCodeSCHEDULINGNOTSETUP, res.Error().Error.Code)
	assert.Equal(t, http.StatusForbidden, a.request(http.MethodGet, "/v1/scheduling/me", tenant.Users[api.RoleViewer].Token, nil).Status)

	var sched api.Schedule
	a.request(http.MethodGet, "/v1/scheduling/me", recruiter, nil).JSON(&sched)
	assert.False(t, sched.Set)
	assert.Len(t, sched.Hours, 5, "weekdays 9 to 5 to start from")

	// Every day 08:00-18:00 in KL, bookable from 2 hours on, 3 days ahead.
	kl, _ := time.LoadLocation("Asia/Kuala_Lumpur")
	var hours []map[string]any
	for d := 1; d <= 7; d++ {
		hours = append(hours, map[string]any{"day": d, "start": "08:00", "end": "18:00"})
	}
	tomorrow := time.Now().In(kl).AddDate(0, 0, 1)
	busyAt := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 10, 0, 0, 0, kl)
	cal := calendarServer(t, busyAt)
	secretURL := cal.URL + "/calendar/ical/me%40gmail.com/private-0123456789abcdef/basic.ics"
	res = a.request(http.MethodPut, "/v1/scheduling/me", recruiter, map[string]any{
		"timeZone": "Asia/Kuala_Lumpur", "hours": hours, "bufferMinutes": 0, "minNoticeHours": 2, "maxDaysAhead": 3, "calendarUrl": secretURL,
	})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	assert.NotContains(t, string(res.Body), "private-0123456789abcdef", "the calendar address is never shown back")
	runCoreJobsUntilIdle(t, a, tenant.ID)
	a.request(http.MethodGet, "/v1/scheduling/me", recruiter, nil).JSON(&sched)
	require.True(t, sched.Set)
	c := sched.Calendar.MustGet()
	assert.Equal(t, api.CalendarFeedStatusOk, c.Status, "%v", c.Error)
	assert.Equal(t, "127.0.0.1", c.Host)
	assert.Equal(t, 1, c.BusyCount)

	link := a.bookingLink(recruiter, map[string]any{
		"candidateId": cand.Id, "title": "Call with the recruiter", "durationMinutes": 30, "location": "https://meet.google.com/abc-defg-hij",
	})
	m := tokenInURL.FindStringSubmatch(link.Url.MustGet())
	require.NotNil(t, m, link.Url.MustGet())
	token := m[1]
	assert.Equal(t, api.BookingStatusOpen, link.Status)

	page := a.publicBooking(token)
	assert.Equal(t, "Test recruiter", page.HostName)
	assert.Equal(t, "Asia/Kuala_Lumpur", page.TimeZone)
	assert.True(t, page.Location.IsNull(), "where is shown once booked")
	require.NotEmpty(t, page.Slots)
	for _, s := range page.Slots {
		assert.False(t, s.Before(busyAt.Add(time.Hour)) && s.Add(30*time.Minute).After(busyAt), "%v is in the busy hour", s)
		assert.True(t, s.After(time.Now().Add(2*time.Hour-time.Minute)), "not before the notice")
	}

	// Book a time; both get an invitation.
	slot := page.Slots[0]
	res = a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{"startsAt": slot})
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	res.JSON(&page)
	assert.Equal(t, api.BookingStatusBooked, page.Status)
	assert.True(t, page.StartsAt.MustGet().Equal(slot))
	assert.Equal(t, "https://meet.google.com/abc-defg-hij", page.Location.MustGet())
	runCoreJobsUntilIdle(t, a, tenant.ID)
	for _, to := range []string{"siti." + tenant.ID + "@mail.test", hostEmail} {
		msgs := mailServer.To(to)
		require.Len(t, msgs, 1, to)
		ics := invitation(t, msgs[0].Data)
		assert.Contains(t, ics, "METHOD:REQUEST")
		assert.Contains(t, ics, "DTSTART:"+slot.UTC().Format("20060102T150405Z"))
		assert.Contains(t, strings.ReplaceAll(ics, "\r\n ", ""), "mailto:"+hostEmail)
		assert.Contains(t, msgs[0].Data, "invite.ics")
	}

	t.Run("a booked time is busy for every other link", func(t *testing.T) {
		other := a.createCandidate(recruiter, map[string]any{"firstName": "Ben", "email": "ben." + tenant.ID + "@mail.test"})
		l2 := a.bookingLink(recruiter, map[string]any{"candidateId": other.Id, "title": "Call", "durationMinutes": 30})
		t2 := tokenInURL.FindStringSubmatch(l2.Url.MustGet())[1]
		for _, s := range a.publicBooking(t2).Slots {
			assert.False(t, s.Equal(slot), "the booked time is not offered again")
		}
		res := a.request(http.MethodPost, "/v1/public/bookings/"+t2, "", map[string]any{"startsAt": slot})
		require.Equal(t, http.StatusConflict, res.Status, "%s", res.Body)
		assert.Equal(t, api.ErrorCodeBOOKINGSLOTTAKEN, res.Error().Error.Code)
	})

	t.Run("the candidate can move it", func(t *testing.T) {
		page := a.publicBooking(token)
		require.True(t, page.CanChange)
		later := page.Slots[len(page.Slots)-1]
		res := a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{"startsAt": later})
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		runCoreJobsUntilIdle(t, a, tenant.ID)
		msgs := mailServer.To(hostEmail)
		require.Len(t, msgs, 2)
		ics := invitation(t, msgs[1].Data)
		assert.Contains(t, ics, "SEQUENCE:2")
		assert.Contains(t, ics, "UID:"+link.Id+"@hiringplug", "the same meeting, updated")
	})

	t.Run("cancelling takes it off both calendars", func(t *testing.T) {
		res := a.request(http.MethodPost, "/v1/booking-links/"+link.Id+"/cancel", recruiter, nil)
		require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
		var l api.BookingLink
		res.JSON(&l)
		assert.Equal(t, api.BookingStatusCancelled, l.Status)
		assert.True(t, l.Url.IsNull(), "the link is not stored")
		runCoreJobsUntilIdle(t, a, tenant.ID)
		msgs := mailServer.To("siti." + tenant.ID + "@mail.test")
		require.Len(t, msgs, 3)
		assert.Contains(t, invitation(t, msgs[2].Data), "METHOD:CANCEL")

		page := a.publicBooking(token)
		assert.Equal(t, api.BookingStatusCancelled, page.Status)
		assert.Empty(t, page.Slots)
		res = a.request(http.MethodPost, "/v1/public/bookings/"+token, "", map[string]any{"startsAt": slot})
		assert.Equal(t, api.ErrorCodeBOOKINGCLOSED, res.Error().Error.Code)
	})

	t.Run("the candidate's links, and nothing for unknown tokens", func(t *testing.T) {
		var list struct{ Items []api.BookingLink }
		a.request(http.MethodGet, "/v1/booking-links?candidateId="+cand.Id, recruiter, nil).JSON(&list)
		require.Len(t, list.Items, 1)
		assert.Equal(t, api.BookingStatusCancelled, list.Items[0].Status)
		assert.Equal(t, http.StatusNotFound, a.request(http.MethodGet, "/v1/public/bookings/"+strings.Repeat("x", 43), "", nil).Status)
	})
}
