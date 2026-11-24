package app

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/interviews"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/pipeline"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/privacy"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/scheduling"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
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

func (a calendarUsers) PrincipalFor(ctx context.Context, tx pgx.Tx, user string) (authz.Principal, bool, error) {
	return a.identity.PrincipalFor(ctx, tx, user)
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
	if err != nil {
		return nil, err
	}
	for _, m := range bookings {
		out = append(out, bookingMeeting(m))
	}
	return out, nil
}
func (a calendarSources) Existing(ctx context.Context, tx pgx.Tx, kind string, ids []string) (map[string]bool, error) {
	switch kind {
	case "interview":
		return a.interviews.StandingIDs(ctx, tx, ids)
	case "booking":
		return a.scheduling.BookedIDs(ctx, tx, ids)
	}
	return map[string]bool{}, nil
}
func interviewMeeting(m interviews.CalendarMeeting) calendars.Meeting {
	return calendars.Meeting{Kind: "interview", ID: m.ID, Title: m.Title, Location: m.Location, Version: m.Version, Users: m.Users, Start: m.Start, End: m.End,
		ApplicationID: m.ApplicationID, Mode: m.Mode, Instructions: m.Instructions, Cancelled: m.Cancelled, Scheduled: m.Scheduled}
}
func bookingMeeting(m scheduling.CalendarMeeting) calendars.Meeting {
	return calendars.Meeting{Kind: "booking", ID: m.ID, Title: m.Title, Location: m.Location, Version: m.Version, Users: []string{m.Host}, Start: m.Start, End: m.End, Cancelled: m.Cancelled}
}

type calendarRecipients struct {
	pipeline   *pipeline.Module
	candidates *candidates.Module
	privacy    *privacy.Module
}

type calendarReminderMail struct{ modules *Modules }

// QueueInterviewReminder queues the candidate's reminder as the organizer's
// email (from their mailbox when connected, replies to them), on the
// candidate's record (their Emails tab, deleted with them).
func (a calendarReminderMail) QueueInterviewReminder(ctx context.Context, tx pgx.Tx, tenant, application, organizer, email string, at time.Time, reminder api.InterviewReminderDelivery) error {
	n := emails.Notice{
		To: []string{email}, Subject: "Interview reminder", Body: "A reminder about your upcoming interview.",
		ApplicationID: &application, DeliverAt: at, InterviewReminder: &reminder, SentBy: &organizer,
	}
	apps, err := a.modules.Pipeline.Briefs(ctx, tx, []string{application})
	if err != nil {
		return err
	}
	if app, ok := apps[application]; ok {
		n.CandidateID = &app.CandidateID
	}
	names, err := a.modules.Identity.UserNames(ctx, tx, []string{organizer})
	if err != nil {
		return err
	}
	n.SenderName = names[organizer]
	return a.modules.Emails.Notify(ctx, tx, tenant, n)
}

func (a calendarReminderMail) ReminderBody(ctx context.Context, tx pgx.Tx, tenant string, reminder api.InterviewReminderDelivery, to []string) (string, error) {
	return a.modules.Calendars.ReminderBody(ctx, tx, tenant, reminder, to)
}

func (a calendarReminderMail) ReminderSent(ctx context.Context, tx pgx.Tx, reminder api.InterviewReminderDelivery) error {
	return a.modules.Calendars.ReminderSent(ctx, tx, reminder)
}

func (a calendarRecipients) InterviewRecipient(ctx context.Context, tx pgx.Tx, applicationID string) (string, bool, error) {
	apps, err := a.pipeline.Briefs(ctx, tx, []string{applicationID})
	if err != nil {
		return "", false, err
	}
	application, ok := apps[applicationID]
	if !ok {
		return "", false, nil
	}
	people, err := a.candidates.Briefs(ctx, tx, []string{application.CandidateID})
	if err != nil {
		return "", false, err
	}
	person, ok := people[application.CandidateID]
	if !ok || person.Email == nil {
		return "", false, nil
	}
	optedOut, err := a.privacy.DoNotContact(ctx, tx, application.CandidateID)
	return *person.Email, optedOut, err
}

// calendarInterviews takes into an interview what its organizer changed in
// Outlook to its invitation.
type calendarInterviews struct{ m *interviews.Module }

func (a calendarInterviews) ChangeFromOutlook(ctx context.Context, tx pgx.Tx, tenant, id, organizer string, version int, c calendars.MeetingChange) (calendars.Meeting, error) {
	change := interviews.OutlookChange{Title: c.Title, Location: c.Location, Instructions: c.Instructions, StartsAt: c.Start}
	if c.Start != nil && c.End != nil {
		length := c.End.Sub(*c.Start)
		if length%time.Minute != 0 {
			return calendars.Meeting{}, apperr.Validation(apperr.FieldErrors{{Field: "durationMinutes", Message: "an interview lasts whole minutes"}})
		}
		minutes := int(length / time.Minute)
		change.Duration = &minutes
	}
	m, err := a.m.ChangeFromOutlook(ctx, tx, tenant, id, organizer, version, change)
	return interviewMeeting(m), err
}
