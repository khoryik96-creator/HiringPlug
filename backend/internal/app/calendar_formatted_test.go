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

const unsafeNotes = `<p><b>Agenda</b></p><ul><li>Open roles</li></ul><p><a href="https://maju.test/brief">Brief</a></p>` +
	`<script>alert(1)</script><img src="https://tracker.test/p.gif" onerror="x()"><p onclick="steal()">Bye</p>`

// An appointment's notes are formatted as written (safe HTML only) and go to
// Outlook as its body; optional attendees go as Outlook's Optional.
func TestAppointmentsKeepFormattedNotesAndOptionalAttendees(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := setupOutlook(t, a)
	connectOutlook(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	input := calendarInput(time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute))
	input["notesHtml"] = unsafeNotes
	delete(input, "notes") // given no plain notes, they are the formatted notes' text
	input["attendees"] = []any{
		map[string]any{"email": "hr@maju.test"},
		map[string]any{"email": "boss@maju.test", "optional": true},
		map[string]any{"email": "HR@maju.test", "optional": true}, // also asked for as required: stays required
	}
	res := a.request(http.MethodPost, "/v1/calendar/events", token, input)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var event api.CalendarEvent
	res.JSON(&event)
	saved := event.NotesHtml.MustGet()
	assert.Contains(t, saved, "<b>Agenda</b>")
	assert.Contains(t, saved, `href="https://maju.test/brief"`)
	for _, bad := range []string{"<script", "onerror", "onclick", "<img", "tracker.test"} {
		assert.NotContains(t, saved, bad)
	}
	assert.Equal(t, "Agenda\nOpen roles\nBrief\nBye", event.Notes, "the plain text, for HiringPlug and Outlook's text")
	require.Len(t, event.Attendees, 2)
	assert.Nil(t, event.Attendees[0].Optional)
	assert.True(t, *event.Attendees[1].Optional)

	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	require.Len(t, f.events, 1)
	for _, sent := range f.events {
		assert.Equal(t, map[string]any{"contentType": "html", "content": saved}, sent["body"])
		assert.Equal(t, []any{
			map[string]any{"emailAddress": map[string]any{"address": "hr@maju.test", "name": ""}, "type": "required"},
			map[string]any{"emailAddress": map[string]any{"address": "boss@maju.test", "name": ""}, "type": "optional"},
		}, sent["attendees"])
	}
	f.mu.Unlock()

	// Plain notes the composer gives (without its signature) are kept beside the formatted ones.
	input["notes"] = "Agenda: open roles"
	res = a.request(http.MethodPost, "/v1/calendar/events", token, input)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	res.JSON(&event)
	assert.Equal(t, "Agenda: open roles", event.Notes)
	assert.Contains(t, event.NotesHtml.MustGet(), "<b>Agenda</b>")
}

// An interview invitation can carry its own formatted message and optional
// attendees (the hiring manager); the candidate stays its one required
// attendee, and a moved interview's invitation goes back to its instructions.
func TestInterviewInvitationsCarryAFormattedMessageAndOptionalAttendees(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	user := tenant.Users[api.RoleRecruiter]
	f := setupOutlook(t, a)
	connectOutlook(t, a, user.Token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	address := "nadia." + tenant.ID + "@example.test"
	iv, _ := scheduleOutlookTestInterview(t, a, user.Token, address, []string{user.ID})
	invite := "/v1/interviews/" + iv.Id + "/outlook-invitation"
	body := inviteInput(iv, false)
	body["bodyHtml"] = `<p>Hi Nadia, <b>see you Monday</b>.</p><script>x()</script>`
	body["optional"] = []string{"manager@client.test", address, "manager@client.test"}
	res := a.request(http.MethodPost, invite, user.Token, body)
	require.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)

	var state api.InterviewInvitation
	a.request(http.MethodGet, invite, user.Token, nil).JSON(&state)
	assert.EqualValues(t, "sent", state.Status, "optional attendees do not make the candidate's address look changed")
	assert.True(t, state.Issue.IsNull())
	f.mu.Lock()
	require.Len(t, f.events, 1)
	for _, sent := range f.events {
		assert.Equal(t, map[string]any{"contentType": "html", "content": "<p>Hi Nadia, <b>see you Monday</b>.</p>"}, sent["body"])
		assert.Equal(t, []any{
			map[string]any{"emailAddress": map[string]any{"address": address, "name": ""}, "type": "required"},
			map[string]any{"emailAddress": map[string]any{"address": "manager@client.test", "name": ""}, "type": "optional"},
		}, sent["attendees"], "the candidate once, as required; the manager once, as optional")
	}
	f.mu.Unlock()

	a.interview(http.MethodPatch, "/v1/interviews/"+iv.Id, user.Token, map[string]any{"version": iv.Version, "startsAt": iv.StartsAt.Add(time.Hour)}, http.StatusOK)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Equal(t, map[string]any{"contentType": "text", "content": "Please bring your portfolio."}, f.lastPatch["body"],
		"no old time stays in a moved interview's message")
	f.mu.Unlock()
}
