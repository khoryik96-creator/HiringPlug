package interviews

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Calendar shares only scheduling facts, never feedback or candidate salaries.
type OutlookCalendar interface {
	SourceChanged(context.Context, pgx.Tx, string, string, string) error
	LockInterview(context.Context, pgx.Tx, string) error
	InterviewInvitation(context.Context, pgx.Tx, authz.Principal, string) (api.InterviewInvitation, error)
	SendInterviewInvitation(context.Context, pgx.Tx, authz.Principal, string, api.InterviewInvitationInput) (api.InterviewInvitation, error)
	InterviewJoinLink(context.Context, pgx.Tx, string, string) (string, error)
	// Conflicts: interviewers busy by their calendars then (the interviews in
	// named aside) and those whose Outlook has stopped syncing.
	Conflicts(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptInterview string, named []string) (busy, stale []string, err error)
}

func (m *Module) UseCalendar(c OutlookCalendar) { m.service.calendar = c }

type CalendarMeeting struct {
	ID, Title, Location               string
	ApplicationID, Mode, Instructions string
	Version                           int
	Users                             []string
	Start, End                        time.Time
	Cancelled, Scheduled              bool
}

func calendarMeeting(row store.Interview) CalendarMeeting {
	location := ""
	if row.Location != nil {
		location = *row.Location
	}
	instructions := ""
	if row.Instructions != nil {
		instructions = *row.Instructions
	}
	return CalendarMeeting{ID: row.ID, Title: row.Title, Location: location, Version: int(row.Version), Users: row.InterviewerIds, Start: row.StartsAt, End: row.EndsAt,
		ApplicationID: row.ApplicationID, Mode: row.Mode, Instructions: instructions, Cancelled: row.Status == "cancelled", Scheduled: row.Status == "scheduled"}
}
func (m *Module) CalendarMeeting(ctx context.Context, tx pgx.Tx, id string) (CalendarMeeting, error) {
	row, err := store.New(tx).GetInterview(ctx, id)
	return calendarMeeting(row), err
}
func (m *Module) CalendarMeetings(ctx context.Context, tx pgx.Tx, user string, from, to time.Time) ([]CalendarMeeting, error) {
	rows, err := store.New(tx).CalendarMeetings(ctx, store.CalendarMeetingsParams{UserID: user, FromTime: from, ToTime: to})
	out := make([]CalendarMeeting, 0, len(rows))
	for _, row := range rows {
		out = append(out, calendarMeeting(row))
	}
	return out, err
}

// StandingIDs are which of these interviews still stand (not deleted or cancelled).
func (m *Module) StandingIDs(ctx context.Context, tx pgx.Tx, ids []string) (map[string]bool, error) {
	rows, err := store.New(tx).StandingInterviewIDs(ctx, ids)
	out := make(map[string]bool, len(rows))
	for _, id := range rows {
		out[id] = true
	}
	return out, err
}
