package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/testutil"
)

// editInOutlook changes the organizer's events as someone would in Outlook
// (Outlook gives each change a new changeKey), then syncs.
func (c candidateFollowUp) editInOutlook(edit func(e map[string]any)) {
	c.t.Helper()
	c.f.mu.Lock()
	for id, e := range c.f.events {
		edit(e)
		e["changeKey"] = fmt.Sprintf("outlook-%s-%d", id, time.Now().UnixNano())
	}
	c.f.mu.Unlock()
	refreshOutlook(c.t, c.a, c.user, c.tenant)
}

func outlookAttendee(address, kind string) map[string]any {
	return map[string]any{"emailAddress": map[string]any{"address": address, "name": ""}, "type": kind}
}

func outlookTime(at time.Time) map[string]any {
	return map[string]any{"dateTime": at.UTC().Format("2006-01-02T15:04:05"), "timeZone": "UTC"}
}

// event is the organizer's HiringPlug calendar event of this kind (the invitation, or a hold).
func (c candidateFollowUp) event(source string) api.CalendarEvent {
	c.t.Helper()
	now := time.Now().UTC()
	res := c.a.request(http.MethodGet, "/v1/calendar/events?from="+url.QueryEscape(now.Add(-24*time.Hour).Format(time.RFC3339))+
		"&to="+url.QueryEscape(now.Add(10*24*time.Hour).Format(time.RFC3339)), c.user, nil)
	require.Equal(c.t, 200, res.Status, "%s", res.Body)
	var list api.CalendarEventList
	res.JSON(&list)
	for _, e := range list.Items {
		if string(e.Source) == source {
			return e
		}
	}
	c.t.Fatalf("no %s event in the calendar", source)
	return api.CalendarEvent{}
}

func (c candidateFollowUp) invitation() api.InterviewInvitation {
	c.t.Helper()
	res := c.a.request(http.MethodGet, c.path+"/outlook-invitation", c.user, nil)
	require.Equal(c.t, 200, res.Status, "%s", res.Body)
	var out api.InterviewInvitation
	res.JSON(&out)
	return out
}

func (c candidateFollowUp) interview() api.Interview {
	c.t.Helper()
	return c.a.interview(http.MethodGet, c.path, c.user, nil, http.StatusOK)
}

func (c candidateFollowUp) patches() int {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	return c.f.patches
}

// stored is the invitation's row as the database has it.
func (c candidateFollowUp) stored(source string) (version, sent int, status, key string) {
	c.t.Helper()
	require.NoError(c.t, testutil.Current.Admin.QueryRow(c.t.Context(),
		"SELECT version,sent_version,sync_status,coalesce(remote_key,'') FROM calendar_event WHERE tenant_id=$1 AND user_id=$2 AND source=$3",
		c.tenant, c.userID, source).Scan(&version, &sent, &status, &key))
	return
}

func emailsOf(e api.CalendarEvent) []string {
	out := []string{}
	for _, a := range e.Attendees {
		out = append(out, strings.ToLower(string(a.Email)))
	}
	return out
}

func TestOutlookPeopleAddedToAnInterviewInvitationShowInHiringPlug(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{})
	patches := c.patches()
	c.editInOutlook(func(e map[string]any) {
		e["attendees"] = append(e["attendees"].([]any), outlookAttendee("panel.guest@example.test", "required"), outlookAttendee("observer@example.test", "optional"))
	})
	event := c.event("interview_invitation")
	assert.ElementsMatch(t, []string{c.email, "panel.guest@example.test", "observer@example.test"}, emailsOf(event))
	assert.False(t, event.OutlookChangedAt.IsNull(), "the event says it was changed in Outlook")
	assert.EqualValues(t, "sent", c.invitation().Status, "a person added in Outlook is not the candidate taken off")
	assert.Equal(t, patches, c.patches(), "nothing is sent back to Outlook")
	version, sent, status, _ := c.stored("interview_invitation")
	assert.Equal(t, version, sent, "Outlook's change is not HiringPlug's to send")
	assert.Equal(t, "synced", status)
	state := c.reply("accepted")
	require.NotNil(t, state.CandidateResponse)
	assert.EqualValues(t, "accepted", *state.CandidateResponse, "the candidate's answer is still theirs")

	c.editInOutlook(func(e map[string]any) {
		kept := []any{}
		for _, a := range e["attendees"].([]any) {
			if a.(map[string]any)["emailAddress"].(map[string]any)["address"] != "panel.guest@example.test" {
				kept = append(kept, a)
			}
		}
		e["attendees"] = kept
	})
	assert.ElementsMatch(t, []string{c.email, "observer@example.test"}, emailsOf(c.event("interview_invitation")))

	moved := c.iv.StartsAt.Add(2 * time.Hour)
	iv := c.interview()
	c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": iv.Version, "startsAt": moved}, http.StatusOK)
	runCoreJobsUntilIdle(t, c.a, c.tenant)
	c.f.mu.Lock()
	assert.Equal(t, moved.Format("2006-01-02T15:04:05"), c.f.lastPatch["start"].(map[string]any)["dateTime"])
	assert.NotContains(t, c.f.lastPatch, "attendees", "a move does not send HiringPlug's list of people over Outlook's")
	for _, e := range c.f.events {
		assert.Len(t, e["attendees"], 2, "the person added in Outlook stays invited")
	}
	c.f.mu.Unlock()
	assert.EqualValues(t, "sent", c.invitation().Status)
}

func TestOutlookTitlePlaceAndNotesChangeTheInterview(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{})
	patches := c.patches()
	c.editInOutlook(func(e map[string]any) {
		e["subject"] = "Final interview with the panel"
		e["location"] = map[string]any{"displayName": "Level 12, Orbit Tower"}
		e["body"] = map[string]any{"contentType": "text", "content": "Bring two forms of ID."}
	})
	iv := c.interview()
	assert.Equal(t, "Final interview with the panel", iv.Title)
	assert.Equal(t, "Level 12, Orbit Tower", iv.Location.MustGet())
	assert.Equal(t, "Bring two forms of ID.", iv.Instructions.MustGet())
	assert.Equal(t, c.iv.Version+1, iv.Version)
	assert.False(t, iv.OutlookChangedAt.IsNull(), "the interview says it was changed in Outlook")
	event := c.event("interview_invitation")
	assert.Equal(t, "Final interview with the panel", event.Title)
	assert.Equal(t, "Level 12, Orbit Tower", event.Location)
	assert.Equal(t, "Bring two forms of ID.", event.Notes)
	assert.Equal(t, patches, c.patches(), "Outlook already sent the update: nothing goes back")
	var audited int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND entity_id=$2 AND action='interview.updated' AND changes->>'via'='outlook' AND actor_id='system' AND changes->>'by'=$3",
		c.tenant, c.iv.Id, c.userID).Scan(&audited))
	assert.Equal(t, 1, audited, "the audit names the organizer who changed it in Outlook")
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND entity_id=$2 AND action='interview.outlook_invitation_changed' AND changes->>'by'=$3",
		c.tenant, c.iv.Id, c.userID).Scan(&audited))
	assert.Equal(t, 1, audited)

	stamp := func() (xmin string, version int) {
		require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
			"SELECT e.xmin::text,i.version FROM calendar_event e JOIN interview i ON i.id=e.source_id WHERE e.tenant_id=$1 AND e.source='interview_invitation'",
			c.tenant).Scan(&xmin, &version))
		return
	}
	xmin, version := stamp()
	refreshOutlook(t, c.a, c.user, c.tenant)
	refreshOutlook(t, c.a, c.user, c.tenant)
	again, versionAgain := stamp()
	assert.Equal(t, xmin, again, "a sync with nothing new changes nothing (no loop)")
	assert.Equal(t, version, versionAgain)
	assert.Equal(t, patches, c.patches())
}

func TestOutlookNotesOfATeamsOrFormattedInvitationStayOutOfInstructions(t *testing.T) {
	t.Parallel()
	t.Run("teams", func(t *testing.T) {
		t.Parallel()
		c := startOutlookInvitation(t, outlookInvite{teams: true})
		c.editInOutlook(func(e map[string]any) {
			e["body"] = map[string]any{"contentType": "text", "content": "Agenda moved to the second hour.\nJoin Microsoft Teams meeting"}
		})
		assert.Contains(t, c.event("interview_invitation").Notes, "Agenda moved")
		assert.Equal(t, "Please bring your portfolio.", c.interview().Instructions.MustGet(), "the Teams join details are not instructions")
	})
	t.Run("formatted", func(t *testing.T) {
		t.Parallel()
		c := startOutlookInvitation(t, outlookInvite{bodyHTML: "<p>Hi Nadia, <b>see you</b> soon.</p>"})
		c.editInOutlook(func(e map[string]any) {
			e["body"] = map[string]any{"contentType": "text", "content": "Rewritten in Outlook."}
		})
		event := c.event("interview_invitation")
		assert.Equal(t, "Rewritten in Outlook.", event.Notes)
		assert.True(t, event.NotesHtml.IsNull(), "Outlook's message replaces HiringPlug's formatted one")
		assert.Equal(t, "Please bring your portfolio.", c.interview().Instructions.MustGet(), "a whole message is not the instructions")

		iv := c.interview()
		c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": iv.Version, "title": "Second interview"}, http.StatusOK)
		runCoreJobsUntilIdle(t, c.a, c.tenant)
		c.f.mu.Lock()
		assert.Equal(t, "Second interview", c.f.lastPatch["subject"])
		assert.NotContains(t, c.f.lastPatch, "body", "the message rewritten in Outlook is kept")
		c.f.mu.Unlock()
	})
}

func TestOutlookDeletedOrCandidateRemovedStillFlagged(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]func(c candidateFollowUp){
		"deleted": func(c candidateFollowUp) {
			c.f.mu.Lock()
			for id := range c.f.events {
				delete(c.f.events, id)
			}
			c.f.mu.Unlock()
			refreshOutlook(c.t, c.a, c.user, c.tenant)
		},
		"cancelled": func(c candidateFollowUp) {
			c.editInOutlook(func(e map[string]any) { e["isCancelled"] = true })
		},
		"candidate removed": func(c candidateFollowUp) {
			c.editInOutlook(func(e map[string]any) {
				e["attendees"] = []any{outlookAttendee("panel.guest@example.test", "required")}
				e["subject"] = "Renamed while the candidate was taken off"
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := startOutlookInvitation(t, outlookInvite{})
			edit(c)
			assert.EqualValues(t, "conflict", c.invitation().Status, "cancelling the interview is HiringPlug's call")
			iv := c.interview()
			assert.EqualValues(t, "scheduled", iv.Status)
			assert.Equal(t, c.iv.Title, iv.Title)
			assert.Equal(t, c.iv.Version, iv.Version)
		})
	}
}

func TestOutlookEditsOfAFormattedInvitationNeverBecomeInstructions(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{bodyHTML: "<p>Dear Nadia, <b>see you</b> on Tuesday at 10:00.</p><p>Regards, Sam</p>"})
	letter := func(at string) func(e map[string]any) {
		return func(e map[string]any) {
			e["body"] = map[string]any{"contentType": "text", "content": "Dear Nadia,\nSee you on Tuesday at " + at + ".\nRegards, Sam"}
		}
	}
	c.editInOutlook(letter("10:00"))
	c.editInOutlook(letter("10:30")) // a second edit: the message no longer looks formatted
	assert.Contains(t, c.event("interview_invitation").Notes, "10:30", "the calendar event shows Outlook's message")
	iv := c.interview()
	assert.Equal(t, "Please bring your portfolio.", iv.Instructions.MustGet(), "a whole letter is never the instructions")
	assert.Equal(t, c.iv.Version, iv.Version)
	assert.True(t, iv.OutlookChangedAt.IsNull(), "the interview itself was not changed")
}

func TestOutlookFillingInTheTeamsMeetingIsNoOrganizerChange(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{lateTeams: true})
	require.True(t, c.invitation().JoinUrl.IsNull(), "Outlook answered the create before it had the Teams meeting")
	patches := c.patches()
	// Exchange adds the meeting on its own: the join link, the join details
	// in the message and its place.
	c.editInOutlook(func(e map[string]any) {
		content := ""
		if body, ok := e["body"].(map[string]any); ok {
			content, _ = body["content"].(string)
		}
		e["onlineMeeting"] = map[string]any{"joinUrl": trialJoin}
		e["body"] = map[string]any{"contentType": "text", "content": content + "\n\nMicrosoft Teams meeting\nJoin on your computer or mobile app\n" + trialJoin}
		e["location"] = map[string]any{"displayName": "Microsoft Teams Meeting"}
	})
	assert.Equal(t, trialJoin, c.invitation().JoinUrl.MustGet(), "the join link is taken")
	iv := c.interview()
	assert.Equal(t, c.iv.Version, iv.Version, "the interview is not changed")
	assert.Equal(t, c.iv.Location, iv.Location)
	assert.Equal(t, c.iv.Instructions, iv.Instructions)
	assert.True(t, iv.OutlookChangedAt.IsNull())
	assert.True(t, c.event("interview_invitation").OutlookChangedAt.IsNull(), "nobody changed the meeting in Outlook")
	var audited int
	require.NoError(t, testutil.Current.Admin.QueryRow(t.Context(),
		"SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND entity_id=$2 AND action IN ('interview.outlook_invitation_changed','interview.updated')",
		c.tenant, c.iv.Id).Scan(&audited))
	assert.Zero(t, audited, "no Outlook change is recorded")
	assert.Equal(t, patches, c.patches(), "nothing is sent to Outlook")

	c.editInOutlook(func(e map[string]any) { e["subject"] = "Final interview with the panel" })
	assert.Equal(t, "Final interview with the panel", c.interview().Title, "a later change by the organizer is still taken")
	assert.Equal(t, c.iv.Location, c.interview().Location)
}

func TestOutlookChangesMadeAsTheTeamsMeetingArrivesAreKept(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{lateTeams: true})
	c.editInOutlook(func(e map[string]any) {
		content := ""
		if body, ok := e["body"].(map[string]any); ok {
			content, _ = body["content"].(string)
		}
		e["onlineMeeting"] = map[string]any{"joinUrl": trialJoin}
		e["body"] = map[string]any{"contentType": "text", "content": content + "\nUse the side entrance.\n\nMicrosoft Teams meeting\nJoin on your computer or mobile app\n" + trialJoin}
		e["location"] = map[string]any{"displayName": "Level 12, Orbit Tower; Microsoft Teams Meeting"}
	})
	assert.Equal(t, trialJoin, c.invitation().JoinUrl.MustGet())
	location, _ := c.interview().Location.Get()
	assert.Equal(t, "Level 12, Orbit Tower; Microsoft Teams Meeting", location, "the place the organizer set is taken")
	assert.Contains(t, c.event("interview_invitation").Notes, "side entrance", "and the words they added")
}

func TestOutlookOnlineMeetingRemovedTakesItsLinkAway(t *testing.T) {
	t.Parallel()
	c := startOutlookInvitation(t, outlookInvite{teams: true})
	require.Equal(t, trialJoin, c.invitation().JoinUrl.MustGet())
	c.editInOutlook(func(e map[string]any) {
		e["onlineMeeting"] = nil
		e["isOnlineMeeting"] = false
		e["body"] = map[string]any{"contentType": "text", "content": "We will meet at the office instead."}
	})
	event := c.event("interview_invitation")
	assert.Equal(t, "We will meet at the office instead.", event.Notes)
	assert.True(t, event.JoinUrl.IsNull(), "the calendar event no longer joins a meeting that is gone")
	assert.False(t, event.Teams, "the event is no longer a Teams meeting")
	assert.True(t, c.invitation().JoinUrl.IsNull())

	moved := c.iv.StartsAt.Add(time.Hour)
	c.a.interview(http.MethodPatch, c.path, c.user, map[string]any{"version": c.interview().Version, "startsAt": moved}, http.StatusOK)
	runCoreJobsUntilIdle(t, c.a, c.tenant)
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	assert.Contains(t, c.f.lastPatch, "start", "the move reaches Outlook")
	assert.NotContains(t, c.f.lastPatch, "isOnlineMeeting", "without asking for a new Teams meeting")
	assert.NotContains(t, c.f.lastPatch, "onlineMeetingProvider")
}
