package calendars

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
)

func syntheticOutlookEvent(t *testing.T, raw string) graphEvent {
	t.Helper()
	var e graphEvent
	require.NoError(t, json.Unmarshal([]byte(raw), &e))
	return e
}

func TestDiffOutlookComparesWithTheLastReceipt(t *testing.T) {
	start := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(45 * time.Minute)
	sent := `{"id":"e1","subject":"Technical interview","location":{"displayName":"Microsoft Teams Meeting"},
		"body":{"contentType":"text","content":"Please bring your portfolio.\r\n"},
		"attendees":[{"emailAddress":{"address":"nadia@example.test"},"type":"required","status":{"response":"none"}},
		{"emailAddress":{"address":"lead@example.test"},"type":"optional"}]}`
	known := syntheticOutlookEvent(t, sent)
	row := store.CalendarEvent{Title: "Technical interview", Location: "", Notes: "Please bring your portfolio.", StartsAt: start, EndsAt: end, Attendees: []byte(`[]`)}

	same := syntheticOutlookEvent(t, sent)
	same.Attendees[0].Status.Response = "accepted"
	assert.True(t, diffOutlook(row, &known, same, start, end).empty(), "a reply only, and Outlook's own Teams location")

	renamed := syntheticOutlookEvent(t, sent)
	renamed.Subject = "Final interview"
	assert.Equal(t, outlookDiff{Title: true}, diffOutlook(row, &known, renamed, start, end))

	spaced := syntheticOutlookEvent(t, sent)
	spaced.Body.Content = "  Please bring your portfolio.  "
	assert.True(t, diffOutlook(row, &known, spaced, start, end).empty(), "spaces around the message are no change")

	reordered := syntheticOutlookEvent(t, sent)
	reordered.Attendees[0], reordered.Attendees[1] = reordered.Attendees[1], reordered.Attendees[0]
	reordered.Attendees[0].EmailAddress.Address = "LEAD@example.test"
	assert.True(t, diffOutlook(row, &known, reordered, start, end).empty(), "the same people in another order or case")

	required := syntheticOutlookEvent(t, sent)
	required.Attendees[1].Type = "required"
	assert.Equal(t, outlookDiff{Attendees: true}, diffOutlook(row, &known, required, start, end), "a Cc made required")

	moved := syntheticOutlookEvent(t, sent)
	assert.Equal(t, outlookDiff{Time: true}, diffOutlook(row, &known, moved, start.Add(time.Hour), end.Add(time.Hour)))

	assert.Equal(t, []string{"title", "attendees"}, outlookDiff{Title: true, Attendees: true}.fields())
}

func TestDiffOutlookWithoutAReceiptComparesWithTheRow(t *testing.T) {
	start := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	row := store.CalendarEvent{Title: "Screening", Location: "Room 4", Notes: "Hello", StartsAt: start, EndsAt: start.Add(time.Hour),
		Attendees: []byte(`[{"email":"nadia@example.test"}]`), RemoteSnapshot: []byte(`{"deleted":true}`)}
	assert.Nil(t, knownEvent(row), "a deletion marker is no receipt")
	e := syntheticOutlookEvent(t, `{"id":"e1","subject":"Screening","location":{"displayName":"Room 4"},"body":{"contentType":"text","content":"Hello"},
		"attendees":[{"emailAddress":{"address":"Nadia@example.test"},"type":"required"}]}`)
	assert.True(t, diffOutlook(row, nil, e, row.StartsAt, row.EndsAt).empty())
	e.Location.DisplayName = "Room 5"
	assert.Equal(t, outlookDiff{Location: true}, diffOutlook(row, nil, e, row.StartsAt, row.EndsAt))
}

func TestPatchKeepsThePeopleOutlookHas(t *testing.T) {
	row := store.CalendarEvent{Source: invitationSource, Attendees: []byte(`[{"email":"nadia@example.test"},{"email":"guest@example.test","optional":true}]`),
		RemoteSnapshot: []byte(`{"id":"e1","attendees":[{"emailAddress":{"address":"guest@example.test"},"type":"optional"},{"emailAddress":{"address":"nadia@example.test"},"type":"required"},{"emailAddress":{"address":"not an address"},"type":"required"}]}`)}
	out, err := patchBody(row, []byte(`{"subject":"x","attendees":[]}`))
	require.NoError(t, err)
	assert.NotContains(t, string(out), "attendees", "the people Outlook has (even one HiringPlug cannot read) stay")
	row.Attendees = []byte(`[{"email":"nadia@example.test"}]`)
	out, err = patchBody(row, []byte(`{"subject":"x","attendees":[]}`))
	require.NoError(t, err)
	assert.Contains(t, string(out), "attendees", "people HiringPlug changed are sent")
	row.Source = "local"
	row.Attendees = []byte(`[{"email":"nadia@example.test"},{"email":"guest@example.test","optional":true}]`)
	out, err = patchBody(row, []byte(`{"subject":"x","attendees":[]}`))
	require.NoError(t, err)
	assert.Contains(t, string(out), "attendees", "an own appointment sends its people as before")
}
