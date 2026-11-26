package scheduling

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
)

type OutlookCalendar interface {
	SourceChanged(context.Context, pgx.Tx, string, string, string) error
	BusyFor(context.Context, pgx.Tx, []string, time.Time, time.Time, string, string) ([]ical.Interval, error)
	Connected(context.Context, pgx.Tx, string) (bool, error)
}

func (m *Module) UseCalendar(c OutlookCalendar) { m.service.calendar = c }

type CalendarMeeting struct {
	ID, Title, Location, Host string
	Version                   int
	Start, End                time.Time
	Cancelled                 bool
}

func calendarMeeting(row store.BookingLink) CalendarMeeting {
	m := CalendarMeeting{ID: row.ID, Title: row.Title, Host: row.HostUserID, Version: int(row.Sequence), Cancelled: row.Status != "booked"}
	// What they gave when booking goes with the place, for the call.
	var where []string
	for _, part := range []*string{row.Location, row.GivenPhone, row.GivenEmail} {
		if part != nil && *part != "" {
			where = append(where, *part)
		}
	}
	m.Location = strings.Join(where, " · ")
	if row.StartsAt != nil {
		m.Start = *row.StartsAt
		m.End = m.Start.Add(time.Duration(row.DurationMinutes) * time.Minute)
	}
	return m
}
func (m *Module) CalendarMeeting(ctx context.Context, tx pgx.Tx, id string) (CalendarMeeting, error) {
	row, err := store.New(tx).CalendarBooking(ctx, id)
	return calendarMeeting(row), err
}
func (m *Module) CalendarMeetings(ctx context.Context, tx pgx.Tx, user string, from, to time.Time) ([]CalendarMeeting, error) {
	rows, err := store.New(tx).CalendarBookings(ctx, store.CalendarBookingsParams{UserID: user, FromTime: from, ToTime: to})
	out := make([]CalendarMeeting, 0, len(rows))
	for _, row := range rows {
		out = append(out, calendarMeeting(row))
	}
	return out, err
}

// BookedIDs are which of these booking links are still booked.
func (m *Module) BookedIDs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]bool, error) {
	rows, err := store.New(tx).BookedLinkIDs(ctx, ids)
	out := make(map[string]bool, len(rows))
	for _, id := range rows {
		out[id] = true
	}
	return out, err
}
