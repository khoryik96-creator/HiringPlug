package app

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
)

type calendarApps struct{ emails *emails.Module }

func (a calendarApps) MicrosoftApp(ctx context.Context, tx pgx.Tx, tenant string) (calendars.Application, error) {
	app, err := a.emails.MicrosoftCalendarApplication(ctx, tx, tenant)
	return calendars.Application{ClientID: app.ClientID, ClientSecret: app.ClientSecret}, err
}

type calendarUsers struct{ identity *identity.Module }

func (a calendarUsers) CalendarUserActive(ctx context.Context, tx pgx.Tx, user string) (bool, error) {
	active, err := a.identity.ActiveUsers(ctx, tx, []string{user})
	return slices.Contains(active[user], api.PermissionCalendarsUse), err
}

type calendarSources struct {
	interviews *interviews.Module
	scheduling *scheduling.Module
}

func (a calendarSources) Meeting(ctx context.Context, tx pgx.Tx, kind, id string) (calendars.Meeting, error) {
	switch kind {
	case "interview":
		m, err := a.interviews.CalendarMeeting(ctx, tx, id)
		return interviewMeeting(m), err
	case "booking":
		m, err := a.scheduling.CalendarMeeting(ctx, tx, id)
		return bookingMeeting(m), err
	}
	return calendars.Meeting{}, apperr.NotFound("meeting")
}
func (a calendarSources) MeetingsFor(ctx context.Context, tx pgx.Tx, user string, from, to time.Time) ([]calendars.Meeting, error) {
	out := []calendars.Meeting{}
	interviews, err := a.interviews.CalendarMeetings(ctx, tx, user, from, to)
	if err != nil {
		return nil, err
	}
	for _, m := range interviews {
		out = append(out, interviewMeeting(m))
	}
	bookings, err := a.scheduling.CalendarMeetings(ctx, tx, user, from, to)
	for _, m := range bookings {
		out = append(out, bookingMeeting(m))
	}
	return out, err
}
func interviewMeeting(m interviews.CalendarMeeting) calendars.Meeting {
	return calendars.Meeting{Kind: "interview", ID: m.ID, Title: m.Title, Location: m.Location, Version: m.Version, Users: m.Users, Start: m.Start, End: m.End, Cancelled: m.Cancelled}
}
func bookingMeeting(m scheduling.CalendarMeeting) calendars.Meeting {
	return calendars.Meeting{Kind: "booking", ID: m.ID, Title: m.Title, Location: m.Location, Version: m.Version, Users: []string{m.Host}, Start: m.Start, End: m.End, Cancelled: m.Cancelled}
}
