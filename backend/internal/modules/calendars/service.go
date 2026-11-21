package calendars

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

type Service struct {
	pool   *pgxpool.Pool
	deps   Deps
	config Config
	now    func() time.Time
	// Slots for Outlook work holding a database connection (see network).
	netSlots chan struct{}
}

func personal(ctx context.Context) (authz.Principal, error) {
	p := authz.MustFromContext(ctx)
	if !strings.HasPrefix(p.UserID, "usr_") {
		return p, apperr.New(api.ErrorCodeFORBIDDEN, "Sign in as a person to use a private calendar; API keys cannot connect personal accounts.")
	}
	return p, nil
}
func ptr[T any](v T) *T { return &v }
func (s *Service) callbackURL() string {
	return strings.TrimRight(s.config.PublicURL, "/") + "/v1/calendar/callback"
}
func (s *Service) statusIn(ctx context.Context, tx pgx.Tx, p authz.Principal) (api.OutlookConnection, error) {
	out := api.OutlookConnection{Status: "disconnected", CallbackUrl: s.callbackURL(), Address: c.OrNull[string](nil), CalendarName: c.OrNull[string](nil), Error: c.OrNull[string](nil), LastSyncAt: c.OrNull[time.Time](nil)}
	if s.deps.Apps != nil {
		app, err := s.deps.Apps.MicrosoftApp(ctx, tx, p.TenantID)
		if err != nil {
			return out, err
		}
		out.Ready = len(s.config.KeysSecret) >= 32 && app.ClientID != "" && app.ClientSecret != ""
	}
	row, err := store.New(tx).CalendarConnection(ctx, p.UserID)
	if database.IsNoRows(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Connected = true
	out.Status = api.OutlookConnectionStatus(row.Status)
	out.Address = c.OrNull(&row.Address)
	out.CalendarName = c.OrNull(&row.CalendarName)
	out.LastSyncAt = c.OrNull(row.SyncedAt)
	out.Error = c.OrNull(row.SyncError)
	out.Version = int(row.Version)
	out.TeamsAvailable = row.TeamsAvailable
	return out, nil
}
func (s *Service) status(ctx context.Context, p authz.Principal) (api.OutlookConnection, error) {
	var out api.OutlookConnection
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error { var err error; out, err = s.statusIn(ctx, tx, p); return err })
	return out, err
}
func (s *Service) list(ctx context.Context, p authz.Principal, from, to time.Time) (api.CalendarEventList, error) {
	out := api.CalendarEventList{Items: []api.CalendarEvent{}}
	if err := checkRange(from, to); err != nil {
		return out, err
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if s.deps.Sources != nil {
			meetings, err := s.deps.Sources.MeetingsFor(ctx, tx, p.UserID, from, to)
			if err != nil {
				return err
			}
			for _, m := range meetings {
				if _, err := s.holdIn(ctx, tx, p.TenantID, p.UserID, m); err != nil {
					return err
				}
			}
		}
		rows, err := store.New(tx).ListCalendarEvents(ctx, store.ListCalendarEventsParams{FromTime: from, ToTime: to})
		if err != nil {
			return err
		}
		if len(rows) > 1000 {
			return apperr.Validation(apperr.FieldErrors{{Field: "to", Message: "this range has more than 1,000 events; choose fewer days"}})
		}
		keys := []sourceKey{}
		for _, row := range rows {
			if row.SourceID != nil && row.Status == "active" {
				keys = append(keys, sourceOf(row.Source, *row.SourceID))
			}
		}
		gone, err := s.gone(ctx, tx, keys) // an interview deleted with its candidate is not shown, and is cleared
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.SourceID != nil && gone[sourceOf(row.Source, *row.SourceID)] {
				continue
			}
			item, err := eventAPI(row)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		out.Connection, err = s.statusIn(ctx, tx, p)
		return err
	})
	return out, err
}
func eventAPI(row store.CalendarEvent) (api.CalendarEvent, error) {
	people := []api.CalendarAttendee{}
	if err := json.Unmarshal(row.Attendees, &people); err != nil {
		return api.CalendarEvent{}, err
	}
	return api.CalendarEvent{Id: row.ID, Version: int(row.Version), Title: row.Title, Notes: row.Notes, NotesHtml: c.OrNull(row.NotesHtml), Location: row.Location, StartsAt: row.StartsAt, EndsAt: row.EndsAt, TimeZone: row.TimeZone, AllDay: row.AllDay, Teams: row.Teams, Attendees: people, Source: api.CalendarEventSource(row.Source), SourceId: c.OrNull(row.SourceID), Status: api.CalendarEventStatus(row.Status), SyncStatus: api.CalendarEventSyncStatus(row.SyncStatus), SyncError: c.OrNull(row.SyncError), WebUrl: c.OrNull(row.WebUrl), JoinUrl: c.OrNull(row.JoinUrl), Editable: row.Source == "local" && row.Status == "active"}, nil
}
func (s *Service) save(ctx context.Context, p authz.Principal, id string, version int, in Input) (api.CalendarEvent, error) {
	var out api.CalendarEvent
	in, err := normalize(in)
	if err != nil {
		return out, err
	}
	if id != "" && (version < 1 || version > math.MaxInt32) {
		return out, apperr.Validation(apperr.FieldErrors{{Field: "version", Message: "reload the appointment"}})
	}
	people, _ := json.Marshal(in.Attendees)
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		conn, err := q.CalendarConnection(ctx, p.UserID)
		connected := err == nil
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		if (len(in.Attendees) > 0 || in.Teams) && !connected {
			return apperr.New(api.ErrorCodeCALENDARNOTCONNECTED, "Connect Outlook before adding attendees or a Teams meeting.")
		}
		if in.Teams && !conn.TeamsAvailable {
			return apperr.Validation(apperr.FieldErrors{{Field: "teams", Message: "this Outlook calendar does not support Teams meetings"}})
		}
		sync := "local"
		if connected {
			sync = "pending"
		}
		var row store.CalendarEvent
		if id == "" {
			row, err = q.InsertCalendarEvent(ctx, store.InsertCalendarEventParams{Title: in.Title, Notes: in.Notes, NotesHtml: orNil(in.NotesHTML), Location: in.Location, StartsAt: in.Start, EndsAt: in.End, TimeZone: in.TimeZone, AllDay: in.AllDay, Teams: in.Teams, Attendees: people, SyncStatus: sync})
		} else {
			before, e := q.CalendarEvent(ctx, id)
			if database.IsNoRows(e) {
				return apperr.NotFound("calendar event")
			}
			if e != nil {
				return e
			}
			if before.Teams && !in.Teams {
				return apperr.Validation(apperr.FieldErrors{{Field: "teams", Message: "a Teams meeting cannot be removed after creation; cancel the appointment instead"}})
			}
			if before.Source != "local" {
				return apperr.New(api.ErrorCodeCALENDAREVENTREADONLY, "Manage this interview or booking in its original HiringPlug screen, or open an external event in Outlook.")
			}
			row, err = q.UpdateCalendarEvent(ctx, store.UpdateCalendarEventParams{ID: id, ExpectedVersion: int32(version), Title: in.Title, Notes: in.Notes, NotesHtml: orNil(in.NotesHTML), Location: in.Location, StartsAt: in.Start, EndsAt: in.End, TimeZone: in.TimeZone, AllDay: in.AllDay, Teams: in.Teams, Attendees: people, SyncStatus: sync}) //nolint:gosec // range checked above
			if database.IsNoRows(err) {
				return apperr.StaleVersion("calendar event")
			}
		}
		if err != nil {
			return err
		}
		if err := managedBusy(ctx, tx, row); err != nil {
			return err
		}
		if connected {
			if err := s.queuePush(ctx, tx, p.TenantID, row.ID, conn.ID); err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "calendar.saved", EntityType: "calendar_event", EntityID: row.ID, Changes: map[string]any{"attendees": len(in.Attendees), "teams": in.Teams}}); err != nil {
			return err
		}
		out, err = eventAPI(row)
		return err
	})
	return out, err
}
func (s *Service) action(ctx context.Context, p authz.Principal, id string, version int, retry bool) (api.CalendarEvent, error) {
	var out api.CalendarEvent
	if version < 1 || version > math.MaxInt32 {
		return out, apperr.StaleVersion("calendar event")
	}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.CalendarEvent(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("calendar event")
		}
		if err != nil {
			return err
		}
		if row.Source == invitationSource || row.Source == "outlook" || (!retry && row.Source != "local") {
			return apperr.New(api.ErrorCodeCALENDAREVENTREADONLY, "Manage this event in its source calendar or HiringPlug screen.")
		}
		conn, err := q.CalendarConnection(ctx, p.UserID)
		connected := err == nil
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		if retry && !connected {
			return apperr.New(api.ErrorCodeCALENDARNOTCONNECTED, "Connect Outlook first.")
		}
		sync := "local"
		if connected {
			sync = "pending"
		}
		if retry {
			row, err = q.RetryCalendarEvent(ctx, store.RetryCalendarEventParams{ID: id, ExpectedVersion: int32(version)}) //nolint:gosec // range checked above
		} else {
			row, err = q.CancelCalendarEvent(ctx, store.CancelCalendarEventParams{ID: id, ExpectedVersion: int32(version), SyncStatus: sync}) //nolint:gosec // range checked above
		} //nolint:gosec // checked above
		if database.IsNoRows(err) {
			return apperr.StaleVersion("calendar event")
		}
		if err != nil {
			return err
		}
		if err := managedBusy(ctx, tx, row); err != nil {
			return err
		}
		if connected {
			if err := s.queuePush(ctx, tx, p.TenantID, id, conn.ID); err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "calendar.changed", EntityType: "calendar_event", EntityID: id, Changes: map[string]any{"retry": retry, "cancelled": !retry}}); err != nil {
			return err
		}
		out, err = eventAPI(row)
		return err
	})
	return out, err
}
func (s *Service) queuePush(ctx context.Context, tx pgx.Tx, tenant, event, connection string) error {
	_, err := jobqueue.Enqueue(ctx, tx, tenant, api.JobKindCalendarPush, api.CalendarPushJob{EventId: event, ConnectionId: connection})
	return err
}
