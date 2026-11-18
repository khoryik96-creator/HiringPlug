package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

func TestCalendarInterviewHoldsFollowMovesAndRemovedPanelMembers(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager)
	user := tenant.Users[api.RoleRecruiter]
	manager := tenant.Users[api.RoleHiringManager]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	company := a.createCompany(user.Token, map[string]any{"name": "Calendar test agency"})
	job := a.createJob(user.Token, map[string]any{"companyId": company.Id, "title": "Engineer"})
	candidate := a.createCandidate(user.Token, map[string]any{"firstName": "Nadia"})
	application := a.addToJob(user.Token, job.Id, candidate.Id)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	body := map[string]any{"title": "Technical interview", "startsAt": start, "durationMinutes": 60, "mode": "video", "interviewerIds": []string{user.ID, manager.ID}}
	res := a.request(http.MethodPost, "/v1/applications/"+application.Id+"/interviews", user.Token, body)
	require.Equal(t, 201, res.Status, "%s", res.Body)
	var iv api.Interview
	res.JSON(&iv)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	require.Equal(t, 1, f.creates)
	for _, event := range f.events {
		assert.Empty(t, event["attendees"], "native interview holds must not send candidate invitations")
	}
	f.mu.Unlock()
	refreshOutlook(t, a, user.Token, tenant.ID)
	// Moving by only fifteen minutes overlaps the old Outlook hold. Its own
	// identity must be excluded, but other busy events must still block it.
	body["version"] = iv.Version
	body["startsAt"] = start.Add(15 * time.Minute)
	res = a.request(http.MethodPatch, "/v1/interviews/"+iv.Id, user.Token, body)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	res.JSON(&iv)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Equal(t, 1, f.creates)
	assert.Equal(t, 1, f.patches)
	f.mu.Unlock()
	body["version"] = iv.Version
	body["interviewerIds"] = []string{manager.ID}
	res = a.request(http.MethodPatch, "/v1/interviews/"+iv.Id, user.Token, body)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Empty(t, f.events, "removing the recruiter removes their Outlook hold")
	f.mu.Unlock()
}

func TestCalendarBookingMovesReuseHoldAndOnlyCandidateGetsICS(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var hours []map[string]any
	for day := 1; day <= 7; day++ {
		hours = append(hours, map[string]any{"day": day, "start": "08:00", "end": "18:00"})
	}
	res := a.request("PUT", "/v1/scheduling/me", user.Token, map[string]any{"timeZone": "Asia/Kuala_Lumpur", "hours": hours, "bufferMinutes": 0, "minNoticeHours": 2, "maxDaysAhead": 3})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	email := "calendar-booking-" + tenant.ID + "@example.test"
	candidate := a.createCandidate(user.Token, map[string]any{"firstName": "Nadia", "email": email})
	link := a.bookingLink(user.Token, map[string]any{"candidateId": candidate.Id, "title": "Screening call", "durationMinutes": 30})
	token := tokenInURL.FindStringSubmatch(link.Url.MustGet())[1]
	page := a.publicBooking(token)
	require.NotEmpty(t, page.Slots)
	slot := page.Slots[0]
	res = a.request("POST", "/v1/public/bookings/"+token, "", map[string]any{"startsAt": slot})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.Len(t, mailServer.To(email), 1, "candidate invitation is retained")
	f.mu.Lock()
	assert.Equal(t, 1, f.creates)
	for _, e := range f.events {
		assert.Empty(t, e["attendees"])
	}
	f.mu.Unlock()
	refreshOutlook(t, a, user.Token, tenant.ID)
	page = a.publicBooking(token)
	require.NotEmpty(t, page.Slots)
	res = a.request("POST", "/v1/public/bookings/"+token, "", map[string]any{"startsAt": page.Slots[len(page.Slots)-1]})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Equal(t, 1, f.creates)
	assert.Equal(t, 1, f.patches)
	f.mu.Unlock()
	res = a.request("POST", "/v1/booking-links/"+link.Id+"/cancel", user.Token, nil)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Empty(t, f.events)
	f.mu.Unlock()
}
