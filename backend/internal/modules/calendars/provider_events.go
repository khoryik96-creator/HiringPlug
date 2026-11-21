package calendars

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
)

type graphTime struct{ DateTime, TimeZone string }

func (t graphTime) instant() (time.Time, error) {
	if value, err := time.Parse(time.RFC3339Nano, t.DateTime); err == nil {
		return value, nil
	}
	loc := time.UTC
	if t.TimeZone != "" && t.TimeZone != "UTC" {
		var err error
		loc, err = time.LoadLocation(t.TimeZone)
		if err != nil {
			return time.Time{}, errors.New("outlook returned an unsupported event time zone")
		}
	}
	value, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", t.DateTime, loc)
	if err != nil {
		return time.Time{}, errors.New("outlook returned an invalid event time")
	}
	return value, nil
}

type graphAttendee struct {
	EmailAddress struct{ Address, Name string }
	Status       *struct{ Response string }
	Type         string
}
type graphEvent struct {
	Body                                                 struct{ ContentType, Content string }
	ID, Subject, BodyPreview, ChangeKey, WebLink, ShowAs string
	TransactionID                                        string `json:"transactionId"`
	ETag                                                 string `json:"@odata.etag"`
	Start, End                                           graphTime
	IsAllDay, IsCancelled, IsOrganizer                   bool
	Location                                             struct{ DisplayName string }
	OnlineMeeting                                        *struct{ JoinURL string }
	Attendees                                            []graphAttendee
}

func (e graphEvent) times() (time.Time, time.Time, error) {
	start, err := e.Start.instant()
	if err != nil {
		return start, time.Time{}, err
	}
	end, err := e.End.instant()
	if err != nil || !end.After(start) {
		return start, end, errors.New("outlook returned an invalid event interval")
	}
	return start, end, nil
}
func (e graphEvent) key() *string {
	v := e.ETag
	if v == "" && e.ChangeKey != "" {
		v = `W/"` + e.ChangeKey + `"`
	}
	if v == "" {
		return nil
	}
	return &v
}
func (e graphEvent) attendees() []api.CalendarAttendee {
	out := make([]api.CalendarAttendee, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		address := strings.TrimSpace(a.EmailAddress.Address)
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address || len(address) > 254 || strings.ContainsAny(address, "\r\n") {
			continue
		}
		name := bounded(a.EmailAddress.Name, 150)
		attendee := api.CalendarAttendee{Email: toEmail(a.EmailAddress.Address), Name: &name}
		if strings.EqualFold(a.Type, "optional") {
			attendee.Optional = ptr(true)
		}
		out = append(out, attendee)
	}
	return out
}
func (e graphEvent) join() *string {
	if e.OnlineMeeting == nil {
		return nil
	}
	return safeWebURL(e.OnlineMeeting.JoinURL)
}
func (s *Service) readEvents(ctx context.Context, c store.CalendarConnection, token string, from, to time.Time) ([]graphEvent, error) {
	path := s.calendarPath(c.CalendarID) + "/calendarView"
	q := url.Values{"startDateTime": {from.UTC().Format(time.RFC3339)}, "endDateTime": {to.UTC().Format(time.RFC3339)}, "$top": {"250"}, "$select": {"id,subject,body,bodyPreview,start,end,isAllDay,isCancelled,isOrganizer,sensitivity,showAs,location,onlineMeeting,webLink,changeKey,attendees,transactionId"}}
	next := s.graph(path) + "?" + q.Encode()
	seen := map[string]bool{}
	ids := map[string]bool{}
	out := []graphEvent{}
	for page := 0; next != ""; page++ {
		if page >= 100 || seen[next] || !s.validNext(next, path) {
			return nil, errors.New("outlook calendar pagination could not be verified; previous busy times were kept")
		}
		seen[next] = true
		raw, err := s.request(ctx, "GET", next, token, nil, "")
		if err != nil {
			return nil, err
		}
		var p struct {
			Value *[]graphEvent `json:"value"`
			Next  string        `json:"@odata.nextLink"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Value == nil {
			return nil, errors.New("outlook returned an invalid calendar page")
		}
		for _, e := range *p.Value {
			if e.ID == "" || ids[e.ID] {
				return nil, errors.New("outlook repeated or omitted an event identity; refresh again")
			}
			if _, _, err := e.times(); err != nil {
				return nil, err
			}
			if len(e.Attendees) > 100 || len(out) >= 10000 {
				return nil, errors.New("outlook calendar exceeds this sync's event limit; previous busy times were kept")
			}
			ids[e.ID] = true
			out = append(out, e)
		}
		next = p.Next
	}
	return out, nil
}
func (s *Service) oneRemote(ctx context.Context, c store.CalendarConnection, id, token string) (graphEvent, error) {
	raw, err := s.request(ctx, "GET", s.graph(s.calendarPath(c.CalendarID)+"/events/"+url.PathEscape(id)), token, nil, "")
	if err != nil {
		return graphEvent{}, err
	}
	var e graphEvent
	if json.Unmarshal(raw, &e) != nil || e.ID == "" {
		return e, errors.New("outlook returned an invalid event")
	}
	return e, nil
}
func eventBody(row store.CalendarEvent) ([]byte, error) {
	var attendees []api.CalendarAttendee
	if err := json.Unmarshal(row.Attendees, &attendees); err != nil {
		return nil, err
	}
	start, end := row.StartsAt.UTC(), row.EndsAt.UTC()
	zone := "UTC"
	if row.AllDay {
		loc, err := time.LoadLocation(row.TimeZone)
		if err != nil {
			return nil, err
		}
		start, end, zone = row.StartsAt.In(loc), row.EndsAt.In(loc), row.TimeZone
	}
	people := make([]map[string]any, 0, len(attendees))
	for _, a := range attendees {
		name := ""
		if a.Name != nil {
			name = *a.Name
		}
		kind := "required"
		if a.Optional != nil && *a.Optional {
			kind = "optional"
		}
		people = append(people, map[string]any{"emailAddress": map[string]string{"address": string(a.Email), "name": name}, "type": kind})
	}
	content := map[string]string{"contentType": "text", "content": row.Notes}
	if row.NotesHtml != nil && *row.NotesHtml != "" {
		content = map[string]string{"contentType": "html", "content": *row.NotesHtml}
	}
	body := map[string]any{"subject": row.Title, "body": content, "start": map[string]string{"dateTime": start.Format("2006-01-02T15:04:05"), "timeZone": zone}, "end": map[string]string{"dateTime": end.Format("2006-01-02T15:04:05"), "timeZone": zone}, "isAllDay": row.AllDay, "location": map[string]string{"displayName": row.Location}, "attendees": people, "transactionId": transactionID(row, row.RemoteGeneration)}
	if row.Teams {
		body["isOnlineMeeting"] = true
		body["onlineMeetingProvider"] = "teamsForBusiness"
	}
	if row.Source == invitationSource {
		body["isReminderOn"] = row.ReminderMinutes > 0
		if row.ReminderMinutes > 0 {
			body["reminderMinutesBeforeStart"] = row.ReminderMinutes
		}
	}
	return json.Marshal(body)
}

// transactionID is the stable ID a create of row carries, so Outlook keeps one
// event however often the create is retried (and a lost answer can be matched).
func transactionID(row store.CalendarEvent, generation int32) string {
	return stableTransactionID(row.ID + ":" + strconv.FormatInt(int64(generation), 10))
}

// patchBody is what a change sends to an event already in Outlook: never its
// transaction ID, and the notes only when they changed (or are formatted) and the event is no
// Teams meeting (its body carries the join details; replacing it can turn the
// online meeting off).
func patchBody(row store.CalendarEvent, body []byte) ([]byte, error) {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return nil, err
	}
	delete(patch, "transactionId")
	var sent graphEvent
	_ = json.Unmarshal(row.RemoteSnapshot, &sent)
	// Formatted notes are always sent (Outlook rewrites HTML, so it cannot be compared).
	formatted := row.NotesHtml != nil && *row.NotesHtml != ""
	if row.Teams || sent.OnlineMeeting != nil || (!formatted && sent.Body.ContentType == "text" && strings.TrimSpace(sent.Body.Content) == strings.TrimSpace(row.Notes)) {
		delete(patch, "body")
	}
	return json.Marshal(patch)
}
func toEmail(s string) openapi_types.Email { return openapi_types.Email(strings.TrimSpace(s)) }
