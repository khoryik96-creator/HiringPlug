package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/humantime"
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
// attendee. When the interview moves, the fields a template filled in follow
// it; text typed by hand stays as written.
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
	body["bodyHtml"] = `<p>Hi Nadia, <b>see you</b> <span data-field="interview.when">Monday 10am</span>.</p><div data-signature="true"><p>Demo</p></div><script>x()</script>`
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
		assert.Equal(t, map[string]any{"contentType": "html", "content": `<p>Hi Nadia, <b>see you</b> <span data-field="interview.when">Monday 10am</span>.</p><div data-signature="true"><p>Demo</p></div>`}, sent["body"],
			"the template's field and the signature keep their marks")
		assert.Equal(t, []any{
			map[string]any{"emailAddress": map[string]any{"address": address, "name": ""}, "type": "required"},
			map[string]any{"emailAddress": map[string]any{"address": "manager@client.test", "name": ""}, "type": "optional"},
		}, sent["attendees"], "the candidate once, as required; the manager once, as optional")
	}
	f.mu.Unlock()

	moved := iv.StartsAt.Add(time.Hour)
	a.interview(http.MethodPatch, "/v1/interviews/"+iv.Id, user.Token, map[string]any{"version": iv.Version, "startsAt": moved}, http.StatusOK)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	sentBody, _ := f.lastPatch["body"].(map[string]any)
	f.mu.Unlock()
	require.NotNil(t, sentBody, "the message changed, so it is sent again")
	assert.Equal(t, "html", sentBody["contentType"])
	assert.Contains(t, sentBody["content"], `<span data-field="interview.when">`+humantime.When(moved, a.deps.Config.TimeZone)+`</span>`, "the time follows the interview")
	assert.NotContains(t, sentBody["content"], "Monday 10am")
	assert.Contains(t, sentBody["content"], "<b>see you</b>", "the rest stays as written")

	// The same first request again (a double click, a lost answer) is answered, and changes nothing.
	again := inviteInput(iv, false)
	again["version"] = iv.Version + 1
	again["bodyHtml"] = "<p>New</p>"
	res = a.request(http.MethodPost, invite, user.Token, again)
	assert.Equal(t, http.StatusAccepted, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	for _, sent := range f.events {
		assert.NotContains(t, sent["body"].(map[string]any)["content"], "New")
	}
	f.mu.Unlock()
}

// What Outlook does to an appointment never undoes HiringPlug's formatted
// notes unless it changed the body itself, and HiringPlug sends the body
// again only when it changed it.
func TestFormattedNotesSurviveOutlookRepliesAndAreSentOnlyWhenChanged(t *testing.T) {
	t.Parallel()
	a := newTestApp(t)
	tenant := testutil.Current.NewTenant(t, api.RoleRecruiter)
	token := tenant.Users[api.RoleRecruiter].Token
	f := setupOutlook(t, a)
	connectOutlook(t, a, token)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	start := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	input := calendarInput(start)
	input["notesHtml"] = "<p><b>Agenda</b></p><ul><li>Open roles</li></ul>"
	input["attendees"] = []any{map[string]any{"email": "hr@maju.test"}}
	res := a.request(http.MethodPost, "/v1/calendar/events", token, input)
	require.Equal(t, http.StatusCreated, res.Status, "%s", res.Body)
	var event api.CalendarEvent
	res.JSON(&event)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	read := func() api.CalendarEvent {
		var list api.CalendarEventList
		a.request(http.MethodGet, calendarRange(start), token, nil).JSON(&list)
		for _, e := range list.Items {
			if e.Id == event.Id {
				return e
			}
		}
		t.Fatalf("appointment %s is missing", event.Id)
		return api.CalendarEvent{}
	}
	refresh := func() {
		require.Equal(t, http.StatusOK, a.request(http.MethodPost, "/v1/calendar/refresh", token, nil).Status)
		runCoreJobsUntilIdle(t, a, tenant.ID)
	}

	// The attendee accepts: Outlook's event changes, its body does not.
	f.mu.Lock()
	for _, e := range f.events {
		e["changeKey"] = "v-accepted"
		e["attendees"] = []any{map[string]any{"emailAddress": map[string]any{"address": "hr@maju.test", "name": ""}, "type": "required", "status": map[string]any{"response": "accepted"}}}
	}
	f.mu.Unlock()
	refresh()
	got := read()
	assert.Equal(t, "<p><b>Agenda</b></p><ul><li>Open roles</li></ul>", got.NotesHtml.MustGet(), "a reply leaves the formatted notes")

	// A change of time alone, with the notes left out, keeps them and does not replace Outlook's body.
	change := calendarInput(start.Add(time.Hour))
	change["version"] = got.Version
	change["notes"] = got.Notes
	change["attendees"] = []any{map[string]any{"email": "hr@maju.test"}}
	res = a.request(http.MethodPut, "/v1/calendar/events/"+event.Id, token, change)
	require.Equal(t, http.StatusOK, res.Status, "%s", res.Body)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.NotContains(t, f.lastPatch, "body", "Outlook's body is not replaced when HiringPlug did not change it")
	f.mu.Unlock()
	got = read()
	assert.Equal(t, "<p><b>Agenda</b></p><ul><li>Open roles</li></ul>", got.NotesHtml.MustGet())

	// New formatted notes are sent.
	change["version"] = got.Version
	change["notesHtml"] = "<p><i>New agenda</i></p>"
	delete(change, "notes")
	require.Equal(t, http.StatusOK, a.request(http.MethodPut, "/v1/calendar/events/"+event.Id, token, change).Status)
	runCoreJobsUntilIdle(t, a, tenant.ID)
	f.mu.Lock()
	assert.Equal(t, map[string]any{"contentType": "html", "content": "<p><i>New agenda</i></p>"}, f.lastPatch["body"])
	// Then someone rewrites the body in Outlook: that wins, as plain text.
	for _, e := range f.events {
		e["changeKey"] = "v-edited"
		e["body"] = map[string]any{"contentType": "text", "content": "Rewritten in Outlook"}
	}
	f.mu.Unlock()
	refresh()
	got = read()
	assert.True(t, got.NotesHtml.IsNull(), "Outlook's own body replaces the formatted notes")
	assert.Equal(t, "Rewritten in Outlook", got.Notes)

	// Notes that grow past the limit once made safe are refused, not a 500.
	long := calendarInput(start)
	long["notesHtml"] = strings.Repeat(`<a href="https://x.test/a">"q"</a>`, 900)
	res = a.request(http.MethodPost, "/v1/calendar/events", token, long)
	assert.Equal(t, http.StatusBadRequest, res.Status, "%s", res.Body)
}
