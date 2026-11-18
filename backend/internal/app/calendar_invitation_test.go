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

const trialJoin = "https://teams.microsoft.com/l/meetup-join/synthetic-interview"

func scheduleOutlookTestInterview(t *testing.T, a *testApp, user, person string, panel []string) (api.Interview, string) {
	t.Helper()
	company := a.createCompany(user, map[string]any{"name": "Orbit Recruitment"})
	job := a.createJob(user, map[string]any{"companyId": company.Id, "title": "Quality Engineer"})
	candidate := a.createCandidate(user, map[string]any{"firstName": "Nadia", "email": person})
	application := a.addToJob(user, job.Id, candidate.Id)
	iv := a.interview(http.MethodPost, "/v1/applications/"+application.Id+"/interviews", user, map[string]any{
		"title": "Technical interview", "startsAt": time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute),
		"durationMinutes": 45, "mode": "video", "interviewerIds": panel, "instructions": "Please bring your portfolio.",
	}, http.StatusCreated)
	return iv, candidate.Id
}

func inviteInput(iv api.Interview, teams bool) map[string]any {
	return map[string]any{"version": iv.Version, "teams": teams, "reminderMinutes": 30}
}

func TestOutlookInterviewInvitationFollowsInterviewAndSharesTeamsLink(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter, api.RoleHiringManager, api.RoleOwner)
	user, manager := tenant.Users[api.RoleRecruiter], tenant.Users[api.RoleHiringManager]
	f := setupOutlook(t, a)
	f.mu.Lock()
	f.trialJoinURL = trialJoin
	f.mu.Unlock()
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	address := "nadia." + tenant.ID + "@example.test"
	iv, _ := scheduleOutlookTestInterview(t, a, user.Token, address, []string{user.ID, manager.ID})
	path := "/v1/interviews/" + iv.Id
	invite := path + "/outlook-invitation"
	read := func(token string) api.InterviewInvitation {
		res := a.request(http.MethodGet, invite, token, nil)
		require.Equal(t, 200, res.Status, "%s", res.Body)
		var state api.InterviewInvitation
		res.JSON(&state)
		return state
	}
	runCoreJobsUntilIdle(t, a, tenant.ID)
	require.True(t, read(user.Token).CanSend)
	require.Equal(t, address, read(user.Token).Recipient.MustGet())
	require.True(t, read(user.Token).TeamsAvailable)
	require.False(t, read(manager.Token).CanSend)

	res := a.request(http.MethodPost, invite, user.Token, inviteInput(iv, true))
	require.Equal(t, 202, res.Status, "%s", res.Body)
	require.Equal(t, 202, a.request(http.MethodPost, invite, user.Token, inviteInput(iv, true)).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	out := read(user.Token)
	require.EqualValues(t, "sent", out.Status)
	require.Equal(t, trialJoin, out.JoinUrl.MustGet())
	require.Equal(t, trialJoin, read(manager.Token).JoinUrl.MustGet())
	other := testutil.Current.NewTenant(t, api.RoleRecruiter).Users[api.RoleRecruiter].Token
	require.Equal(t, 404, a.request(http.MethodGet, invite, other, nil).Status)
	require.Equal(t, 422, a.request(http.MethodPost, invite, tenant.Users[api.RoleOwner].Token, inviteInput(iv, true)).Status)

	f.mu.Lock()
	require.Len(t, f.events, 1, "the organizer's native invitation replaces their private hold")
	var remoteID string
	for id, event := range f.events {
		remoteID = id
		assert.Equal(t, true, event["isOnlineMeeting"])
		assert.Equal(t, true, event["isReminderOn"])
		assert.Equal(t, float64(30), event["reminderMinutesBeforeStart"])
		assert.Equal(t, []any{map[string]any{"emailAddress": map[string]any{"address": address, "name": ""}, "type": "required"}}, event["attendees"])
	}
	f.mu.Unlock()

	iv = a.interview(http.MethodPatch, path, user.Token, map[string]any{"version": iv.Version, "startsAt": iv.StartsAt.Add(15 * time.Minute)}, http.StatusOK)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Len(t, f.events, 1)
	assert.Contains(t, f.events, remoteID)
	assert.Equal(t, iv.StartsAt.Format("2006-01-02T15:04:05"), f.lastPatch["start"].(map[string]any)["dateTime"])
	assert.NotContains(t, f.lastPatch, "body", "Teams join details are preserved")
	f.mu.Unlock()

	_, err := testutil.Current.Admin.Exec(t.Context(), "UPDATE job SET run_after=now() WHERE tenant_id=$1 AND kind='interview.remind'", tenant.ID)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	reminders := mailServer.To("hiring_manager." + tenant.ID + "@test.local")
	require.Len(t, reminders, 1)
	assert.Contains(t, strings.ReplaceAll(reminders[0].Data, "=\r\n", ""), trialJoin)
	assert.Empty(t, mailServer.To(address), "normal interviewer reminders do not send a second candidate invite")

	a.interview(http.MethodPatch, path, user.Token, map[string]any{"version": iv.Version, "status": "cancelled"}, http.StatusOK)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	out = read(user.Token)
	require.EqualValues(t, "cancelled", out.Status)
	require.True(t, out.JoinUrl.IsNull())
	f.mu.Lock()
	assert.Empty(t, f.events)
	f.mu.Unlock()
}

func TestOutlookInterviewInvitationRechecksConsentAndOrganizingAccount(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	iv, candidateID := scheduleOutlookTestInterview(t, a, user.Token, "nadia@example.test", []string{user.ID})
	path := "/v1/interviews/" + iv.Id + "/outlook-invitation"
	res := a.request(http.MethodPost, path, user.Token, inviteInput(iv, false))
	require.Equal(t, 202, res.Status, "%s", res.Body)
	a.setConsent(user.Token, candidateID, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var out api.InterviewInvitation
	a.request(http.MethodGet, path, user.Token, nil).JSON(&out)
	require.EqualValues(t, "failed", out.Status)
	require.Contains(t, out.Issue.MustGet(), "opted out")
	require.Equal(t, 422, a.request(http.MethodPost, path, user.Token, inviteInput(iv, false)).Status)
	f.mu.Lock()
	for _, event := range f.events {
		assert.Empty(t, event["attendees"], "withdrawal blocks the attendee write even after scheduling")
	}
	f.account = "different-account"
	f.mu.Unlock()
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	a.setConsent(user.Token, candidateID, map[string]any{"expectedVersion": 1, "status": "given", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
	res = a.request(http.MethodPost, path, user.Token, inviteInput(iv, false))
	assert.Equal(t, 422, res.Status, "%s", res.Body)
	assert.Contains(t, string(res.Body), "original Outlook calendar")
}

func TestOutlookInterviewCancelledBeforeSendingDoesNotInvite(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	iv, _ := scheduleOutlookTestInterview(t, a, user.Token, "nadia@example.test", []string{user.ID})
	path := "/v1/interviews/" + iv.Id
	require.Equal(t, 202, a.request(http.MethodPost, path+"/outlook-invitation", user.Token, inviteInput(iv, false)).Status)
	a.interview(http.MethodPatch, path, user.Token, map[string]any{"version": iv.Version, "status": "cancelled"}, http.StatusOK)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	var out api.InterviewInvitation
	res := a.request(http.MethodGet, path+"/outlook-invitation", user.Token, nil)
	require.Equal(t, 200, res.Status, "%s", res.Body)
	res.JSON(&out)
	require.EqualValues(t, "cancelled", out.Status)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Zero(t, f.creates, "the cancelled interview never reaches Outlook as an invitation")
}
