package calendars

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
)

// What changed in Outlook since its last receipt, and how people compare.
// It works on stored rows and contract types, so it is not in domain.go.

// outlookDiff is what changed in Outlook since its last receipt.
type outlookDiff struct{ Title, Location, Body, Attendees, Time bool }

func (d outlookDiff) empty() bool { return d == outlookDiff{} }

// fields names what changed, for the audit (never the values).
func (d outlookDiff) fields() []string {
	out := []string{}
	for _, f := range []struct {
		on   bool
		name string
	}{{d.Title, "title"}, {d.Location, "location"}, {d.Body, "notes"}, {d.Attendees, "attendees"}, {d.Time, "time"}} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return out
}

// diffOutlook compares an event as Outlook has it now with known, the last
// receipt (Outlook rewrites what it is sent: a Teams location, an HTML
// message as text, spaces), or with the row when there is none. The time is
// compared with the row's, as HiringPlug schedules it.
func diffOutlook(row store.CalendarEvent, known *graphEvent, e graphEvent, start, end time.Time) outlookDiff {
	d := outlookDiff{Time: e.IsAllDay != row.AllDay || !start.Equal(row.StartsAt) || !end.Equal(row.EndsAt)}
	body := strings.TrimSpace(e.Body.Content)
	if known != nil {
		d.Title = known.Subject != e.Subject
		d.Location = strings.TrimSpace(known.Location.DisplayName) != strings.TrimSpace(e.Location.DisplayName)
		d.Body = strings.TrimSpace(known.Body.Content) != body
		d.Attendees = !sameGraphAttendees(known.Attendees, e.Attendees)
		return d
	}
	d.Title = row.Title != e.Subject
	d.Location = row.Location != strings.TrimSpace(e.Location.DisplayName)
	d.Body = row.NotesHtml == nil && strings.TrimSpace(row.Notes) != body // a formatted message cannot be compared
	var people []api.CalendarAttendee
	_ = json.Unmarshal(row.Attendees, &people)
	d.Attendees = !sameAttendeeSet(attendeeSet(people), graphAttendeeSet(e.Attendees))
	return d
}

// teamsArrived: Outlook filled in the Teams meeting HiringPlug asked for after
// the last receipt. It writes the join details into the message and may set
// the place, which the organizer did not change.
func teamsArrived(row store.CalendarEvent, known *graphEvent, e graphEvent) bool {
	return row.Teams && known != nil && known.OnlineMeeting == nil && e.OnlineMeeting != nil
}

// teamsPlace is the place Outlook gives a Teams meeting.
const teamsPlace = "Microsoft Teams Meeting"

// withoutTeamsArrival leaves out of d what Outlook itself added when it filled
// in the Teams meeting since known: its Teams place (instead of the place, or
// after it) and the join details it appends to the message. Anything else
// changed meanwhile is the organizer's and stays.
func withoutTeamsArrival(d outlookDiff, known, e graphEvent) outlookDiff {
	place := strings.TrimSpace(e.Location.DisplayName)
	kept := []string{}
	for _, part := range strings.Split(place, ";") {
		if part = strings.TrimSpace(part); part != "" && !strings.EqualFold(part, teamsPlace) {
			kept = append(kept, part)
		}
	}
	if strings.EqualFold(place, teamsPlace) || strings.Join(kept, "; ") == strings.TrimSpace(known.Location.DisplayName) {
		d.Location = false
	}
	if added, ok := strings.CutPrefix(strings.TrimSpace(e.Body.Content), strings.TrimSpace(known.Body.Content)); ok && joinDetails(added, e) {
		d.Body = false
	}
	return d
}

// joinDetails: text added to the end of a message is the Teams join block
// Outlook appends (after its line of underscores, if any), not the
// organizer's words.
func joinDetails(added string, e graphEvent) bool {
	block := strings.TrimLeft(added, " _\t\r\n")
	if block == "" {
		return false
	}
	if e.OnlineMeeting != nil && e.OnlineMeeting.JoinURL != "" && strings.HasPrefix(block, e.OnlineMeeting.JoinURL) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(block), "microsoft teams")
}

// knownEvent is the event's last receipt from Outlook, if it has a usable one.
func knownEvent(row store.CalendarEvent) *graphEvent {
	var known graphEvent
	if len(row.RemoteSnapshot) == 0 || json.Unmarshal(row.RemoteSnapshot, &known) != nil || known.ID == "" {
		return nil
	}
	return &known
}

// Attendees are the same people with the same roles (required or optional),
// in any order and letter case: replies change only their responses.
func sameGraphAttendees(a, b []graphAttendee) bool {
	return sameAttendeeSet(graphAttendeeSet(a), graphAttendeeSet(b))
}

func sameAttendees(people []api.CalendarAttendee, raw []byte) bool {
	var stored []api.CalendarAttendee
	if json.Unmarshal(raw, &stored) != nil {
		return false
	}
	return sameAttendeeSet(attendeeSet(people), attendeeSet(stored))
}

func graphAttendeeSet(people []graphAttendee) map[string]bool {
	out := make(map[string]bool, len(people))
	for _, p := range people {
		out[strings.ToLower(strings.TrimSpace(p.EmailAddress.Address))] = strings.EqualFold(p.Type, "optional")
	}
	return out
}

func attendeeSet(people []api.CalendarAttendee) map[string]bool {
	out := make(map[string]bool, len(people))
	for _, p := range people {
		out[strings.ToLower(strings.TrimSpace(string(p.Email)))] = p.Optional != nil && *p.Optional
	}
	return out
}

func sameAttendeeSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for address, optional := range a {
		if other, ok := b[address]; !ok || other != optional {
			return false
		}
	}
	return true
}
