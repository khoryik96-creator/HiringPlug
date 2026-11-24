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

type candidateFollowUp struct {
	a                              *testApp
	t                              *testing.T
	f                              *outlookFixture
	tenant, user, email, candidate string
	userID, organizer              string // the organizer's id and name
	iv                             api.Interview
	path                           string
	managerID                      string // the hiring manager on the panel, when asked for
}

// outlookInvite is how a test's interview invitation is sent.
type outlookInvite struct {
	teams    bool
	bodyHTML string // a formatted message
	panel    bool   // a hiring manager interviews too
	// lateTeams asks for Teams, but Outlook answers the create before it has
	// the meeting (its join details arrive later).
	lateTeams bool
}

func startCandidateFollowUp(t *testing.T) candidateFollowUp {
	t.Helper()
	return startOutlookInvitation(t, outlookInvite{teams: true})
}

// startOutlookInvitation sends an interview invitation (with the candidate's
// email reminder on) from a recruiter's connected Outlook.
func startOutlookInvitation(t *testing.T, o outlookInvite) candidateFollowUp {
	t.Helper()
	a := newTestApp(t)
	roles := []api.Role{api.RoleRecruiter}
	if o.panel {
		roles = append(roles, api.RoleHiringManager)
	}
	tenant := testutil.Current.NewTenant(t, roles...)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	if o.teams {
		f.mu.Lock()
		f.trialJoinURL = trialJoin
		f.mu.Unlock()
	}
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	email := "nadia." + tenant.ID + "@example.test"
	panel, manager := []string{user.ID}, ""
	if o.panel {
		manager = tenant.Users[api.RoleHiringManager].ID
		panel = append(panel, manager)
	}
	iv, candidate := scheduleOutlookTestInterview(t, a, user.Token, email, panel)
	path := "/v1/interviews/" + iv.Id
	in := inviteInput(iv, o.teams || o.lateTeams)
	in["candidateReminder"] = true
	if o.bodyHTML != "" {
		in["bodyHtml"] = o.bodyHTML
	}
	res := a.request(http.MethodPost, path+"/outlook-invitation", user.Token, in)
	require.Equal(t, 202, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	var name string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), "SELECT name FROM app_user WHERE id=$1", user.ID).Scan(&name))
	return candidateFollowUp{a, t, f, tenant.ID, user.Token, email, candidate, user.ID, name, iv, path, manager}
}

func (c candidateFollowUp) reply(status string) api.InterviewInvitation {
	c.t.Helper()
	c.f.mu.Lock()
	for _, event := range c.f.events {
		for _, attendee := range event["attendees"].([]any) {
			attendee.(map[string]any)["status"] = map[string]string{"response": status}
		}
		event["changeKey"] = status
	}
	c.f.mu.Unlock()
	refreshOutlook(c.t, c.a, c.user, c.tenant)
	res := c.a.request(http.MethodGet, c.path+"/outlook-invitation", c.user, nil)
	require.Equal(c.t, 200, res.Status, "%s", res.Body)
	var out api.InterviewInvitation
	res.JSON(&out)
	return out
}

func (c candidateFollowUp) deliver() {
	c.t.Helper()
	_, err := testutil.Current.Admin.Exec(c.t.Context(),
		"UPDATE job SET run_after=now() WHERE tenant_id=$1 AND kind='email.interview_reminder' AND status='pending'", c.tenant)
	require.NoError(c.t, err)
	runCoreJobsUntilIdle(c.t, c.a, c.tenant)
}

func TestOutlookInterviewCandidateRSVPAndMovingReminder(t *testing.T) {
	t.Parallel()
	c := startCandidateFollowUp(t)
	var runAfter time.Time
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT run_after FROM job WHERE tenant_id=$1 AND kind='email.interview_reminder'", c.tenant).Scan(&runAfter))
	require.WithinDuration(t, c.iv.StartsAt.Add(-24*time.Hour), runAfter, time.Second)
	for remote, expected := range map[string]string{"notResponded": "not_responded", "tentativelyAccepted": "tentative", "accepted": "accepted"} {
		state := c.reply(remote)
		require.NotNil(t, state.CandidateResponse)
		require.EqualValues(t, expected, *state.CandidateResponse)
	}
	c.reply("accepted")
	state := c.reply("organizer") // an answer HiringPlug does not map
	require.NotNil(t, state.CandidateResponse, "an answer we do not know leaves the last one")
	require.EqualValues(t, "accepted", *state.CandidateResponse)
	res := c.a.request(http.MethodPatch, c.path, c.user, map[string]any{
		"version": c.iv.Version, "startsAt": c.iv.StartsAt.Add(2 * time.Hour), "title": "Updated technical interview",
	})
	require.Equal(t, 200, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, c.a, c.tenant)
	c.deliver()

	items := mailServer.To(c.email)
	require.Len(t, items, 1, "the original scheduled reminder was superseded")
	assert.Contains(t, items[0].Data, "Updated technical interview")
	assert.Contains(t, strings.ReplaceAll(items[0].Data, "=\r\n", ""), trialJoin)
	_, err := testutil.Current.Admin.Exec(t.Context(),
		"UPDATE job SET status='pending',run_after=now() WHERE tenant_id=$1 AND kind='email.interview_reminder'", c.tenant)
	require.NoError(t, err)
	runCoreJobsUntilIdle(t, c.a, c.tenant)
	assert.Len(t, mailServer.To(c.email), 1, "a retry cannot send a second reminder")
}

func TestOutlookInterviewCandidateReminderIsTheOrganizersEmailOnTheCandidate(t *testing.T) {
	t.Parallel()
	c := startCandidateFollowUp(t)
	c.deliver()
	require.Len(t, mailServer.To(c.email), 1)
	var candidate, sentBy, sender string
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT coalesce(candidate_id,''),sent_by,sender_name FROM email WHERE tenant_id=$1 AND $2=ANY(to_addresses)", c.tenant, c.email).Scan(&candidate, &sentBy, &sender))
	assert.Equal(t, c.candidate, candidate, "the reminder is on the candidate's record (their Emails tab, deleted with them)")
	assert.Equal(t, c.userID, sentBy, "the organizer sends the reminder, not the server")
	assert.Equal(t, c.organizer, sender)
}

func TestOutlookInterviewCandidateReminderChecksEligibilityAtDelivery(t *testing.T) {
	t.Parallel()
	t.Run("declined", func(t *testing.T) {
		t.Parallel()
		c := startCandidateFollowUp(t)
		state := c.reply("declined")
		require.NotNil(t, state.CandidateResponse)
		require.EqualValues(t, "declined", *state.CandidateResponse)
		c.deliver()
		require.Empty(t, mailServer.To(c.email))
	})
	t.Run("consent withdrawn", func(t *testing.T) {
		t.Parallel()
		c := startCandidateFollowUp(t)
		c.a.setConsent(c.user, c.candidate, map[string]any{"expectedVersion": 0, "status": "withdrawn", "method": "email", "day": klDay(t, 0)}, http.StatusOK)
		c.deliver()
		require.Empty(t, mailServer.To(c.email))
	})
	t.Run("turned off", func(t *testing.T) {
		t.Parallel()
		c := startCandidateFollowUp(t)
		res := c.a.request(http.MethodPut, c.path+"/outlook-reminder", c.user, map[string]any{"version": c.iv.Version, "enabled": false})
		require.Equal(t, 200, res.Status, "%s", res.Body)
		c.deliver()
		require.Empty(t, mailServer.To(c.email))
		var kept, skipped int
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(), `SELECT
			(SELECT count(*) FROM email WHERE tenant_id=$1),
			(SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action='email.reminder_skipped')`, c.tenant).Scan(&kept, &skipped))
		assert.Zero(t, kept, "a reminder no longer due is not listed as a failed email")
		assert.Equal(t, 1, skipped)
	})
	t.Run("cancelled", func(t *testing.T) {
		t.Parallel()
		c := startCandidateFollowUp(t)
		res := c.a.request(http.MethodPatch, c.path, c.user, map[string]any{"version": c.iv.Version, "status": "cancelled"})
		require.Equal(t, 200, res.Status, "%s", res.Body)
		c.deliver()
		require.Empty(t, mailServer.To(c.email))
	})
}
