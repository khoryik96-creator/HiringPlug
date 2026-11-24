package calendars

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

// Attendee is independent of the HTTP contract and provider representations.
type Attendee struct {
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	Optional bool    `json:"optional,omitempty"` // Outlook's Optional: the meeting's Cc
}

// Input is deliberately independent of Graph's representation.
type Input struct {
	Title, Notes, Location, TimeZone string
	NotesHTML                        *string // formatted notes, cleaned in normalize (nil: unchanged on a change; ""/empty: none)
	Start, End                       time.Time
	AllDay, Teams                    bool
	Attendees                        []Attendee
}

func normalize(in Input) (Input, error) {
	var errs apperr.FieldErrors
	in.Title = strings.TrimSpace(in.Title)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.NotesHTML != nil {
		safe, text := cleanNotes(*in.NotesHTML)
		in.NotesHTML = &safe
		if len(safe) > maxNotesHTML { // making it safe can lengthen it (links get rel, quotes are escaped)
			errs.Add("notesHtml", "is too long: shorten the notes")
		}
		if in.Notes == "" && safe != "" { // the notes as text, unless the composer gave them (without its signature)
			in.Notes = bounded(text, 4000)
		}
	}
	in.Location = strings.TrimSpace(in.Location)
	if in.Title == "" {
		errs.Add("title", "enter a title")
	}
	if in.Start.IsZero() || !in.End.After(in.Start) || in.End.Sub(in.Start) > 31*24*time.Hour {
		errs.Add("endsAt", "must be after the start and within 31 days")
	}
	loc, err := time.LoadLocation(in.TimeZone)
	if err != nil {
		errs.Add("timeZone", "choose a valid time zone")
	}
	if in.AllDay && loc != nil {
		a, b := in.Start.In(loc), in.End.In(loc)
		if a.Hour() != 0 || a.Minute() != 0 || a.Second() != 0 || a.Nanosecond() != 0 || b.Hour() != 0 || b.Minute() != 0 || b.Second() != 0 || b.Nanosecond() != 0 {
			errs.Add("allDay", "all-day events must start and end at midnight in the selected time zone")
		}
	}
	seen := map[string]int{} // where each address is in out
	out := make([]Attendee, 0, len(in.Attendees))
	for _, a := range in.Attendees {
		address := strings.ToLower(strings.TrimSpace(a.Email))
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || strings.ContainsAny(address, "\r\n") {
			errs.Add("attendees", "enter valid email addresses")
			continue
		}
		if i, ok := seen[address]; ok {
			out[i].Optional = out[i].Optional && a.Optional // required, if asked for both ways
			continue
		}
		seen[address] = len(out)
		a.Email = address
		out = append(out, a)
	}
	in.Attendees = out
	return in, errs.Err()
}
func checkRange(from, to time.Time) error {
	if !to.After(from) || to.Sub(from) > 62*24*time.Hour {
		return apperr.Validation(apperr.FieldErrors{{Field: "to", Message: "choose a range of up to 62 days after the start"}})
	}
	return nil
}
func stableTransactionID(id string) string {
	h := sha256.Sum256([]byte("hiringplug-calendar:" + id))
	raw := hex.EncodeToString(h[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[:8], raw[8:12], raw[12:16], raw[16:20], raw[20:])
}
func safeWebURL(raw string) *string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if host != "outlook.office.com" && host != "outlook.office365.com" && host != "outlook.live.com" && host != "teams.microsoft.com" && host != "teams.live.com" {
		return nil
	}
	return &raw
}
func bounded(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

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
