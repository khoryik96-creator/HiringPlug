package interviews

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Calendar shares only scheduling facts, never feedback or candidate salaries.
type OutlookCalendar interface {
	SourceChanged(context.Context, pgx.Tx, string, string, string) error
	LockInterview(context.Context, pgx.Tx, string) error
	InterviewInvitation(context.Context, pgx.Tx, authz.Principal, string) (api.InterviewInvitation, error)
	SendInterviewInvitation(context.Context, pgx.Tx, authz.Principal, string, api.InterviewInvitationInput) (api.InterviewInvitation, error)
	SetInterviewReminder(context.Context, pgx.Tx, authz.Principal, string, api.InterviewOutlookReminderInput) (api.InterviewInvitation, error)
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

// ChangeFromOutlook takes into a scheduled interview what its organizer
// changed in Outlook to its invitation (title, place, instructions, time), in
// the calendar refresh's transaction, which holds the interview's calendar
// lock. Nothing is sent back to Outlook and no calendar job is queued here:
// the calendars module, which called, does both as needed. The audit entry is
// an ordinary update (webhooks and automations follow it) marked via Outlook
// and by the organizer who changed it there (the refresh itself runs as the
// system).
func (m *Module) ChangeFromOutlook(ctx context.Context, tx pgx.Tx, tenant, id, organizer string, version int, c OutlookChange) (CalendarMeeting, error) {
	s := m.service
	q := store.New(tx)
	row, err := getRow(ctx, q, id)
	if err != nil {
		return CalendarMeeting{}, err
	}
	if int(row.Version) != version {
		return calendarMeeting(row), apperr.StaleVersion("interview")
	}
	if row.Status != "scheduled" {
		return calendarMeeting(row), apperr.New(api.ErrorCodeINTERVIEWNOTALLOWED, "The interview is no longer scheduled.")
	}
	before := detailsOf(row)
	after, errs := ApplyOutlookChange(before, c)
	if err := errs.Err(); err != nil {
		return calendarMeeting(row), err
	}
	changes := audit.Diff(fields(before), fields(after))
	if _, ok := changes["instructionsLength"]; !ok && text(before.Instructions) != text(after.Instructions) {
		changes["instructionsLength"] = map[string]any{"from": len(text(before.Instructions)), "to": len(text(after.Instructions)), "changed": true}
	}
	if len(changes) == 0 {
		return calendarMeeting(row), nil
	}
	updated, err := q.UpdateInterviewFromOutlook(ctx, store.UpdateInterviewFromOutlookParams{
		ID: id, ExpectedVersion: row.Version, Title: after.Title, StartsAt: after.StartsAt, EndsAt: after.Ends(),
		DurationMinutes: int32(after.Duration), //nolint:gosec // 5..480, checked above
		Location:        after.Location, Instructions: after.Instructions,
	})
	if database.IsNoRows(err) {
		return calendarMeeting(row), apperr.StaleVersion("interview")
	}
	if err != nil {
		return calendarMeeting(row), err
	}
	changes["via"], changes["by"] = "outlook", organizer
	if err := audit.Record(ctx, tx, audit.Entry{Action: "interview.updated", EntityType: "interview", EntityID: id, Changes: changes}); err != nil {
		return calendarMeeting(row), err
	}
	if !after.StartsAt.Equal(before.StartsAt) {
		// Moved: remind of the new time (the old reminder sees the change and does nothing).
		if err := s.queueReminder(ctx, tx, tenant, id, after); err != nil {
			return calendarMeeting(row), err
		}
	}
	return calendarMeeting(updated), nil
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
