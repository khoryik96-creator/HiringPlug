package calendars

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/humantime"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

const candidateReminderLead = 24 * time.Hour
const candidateReminderCutoff = time.Hour

func candidateAttendee(row store.CalendarEvent) (string, error) {
	var people []api.CalendarAttendee
	if err := json.Unmarshal(row.Attendees, &people); err != nil {
		return "", err
	}
	if len(people) == 1 {
		return string(people[0].Email), nil
	}
	return "", nil
}

// A refresh maps the pinned attendee only. The organizer's own response and
// other attendees' responses are never published as the candidate's answer.
func saveCandidateResponse(ctx context.Context, q *store.Queries, row store.CalendarEvent, e graphEvent) (bool, error) {
	if row.Source != invitationSource || e.IsCancelled || e.Attendees == nil {
		return false, nil
	}
	email, err := candidateAttendee(row)
	if err != nil {
		return false, err
	}
	var response *string
	found := false
	for _, person := range e.Attendees {
		if email == "" || !strings.EqualFold(email, strings.TrimSpace(person.EmailAddress.Address)) {
			continue
		}
		found = true
		if person.Status == nil {
			return false, nil // This reply did not include response information.
		}
		switch person.Status.Response {
		case "accepted":
			response = ptr("accepted")
		case "tentativelyAccepted":
			response = ptr("tentative")
		case "declined":
			response = ptr("declined")
		case "none", "notResponded":
			response = ptr("not_responded")
		}
		break
	}
	return !found, q.SaveCandidateResponse(ctx, store.SaveCandidateResponseParams{ID: row.ID, Response: response})
}

// A reply can advance Outlook's ETag. Accept it only while all content
// relevant to future writes still matches the last Outlook receipt.
func responseOnly(row store.CalendarEvent, observed graphEvent) bool {
	var sent graphEvent
	if json.Unmarshal(row.RemoteSnapshot, &sent) != nil ||
		sent.Subject != observed.Subject || sent.Body != observed.Body || sent.Location != observed.Location ||
		sent.IsAllDay != observed.IsAllDay || len(sent.Attendees) != 1 || len(observed.Attendees) != 1 {
		return false
	}
	return strings.EqualFold(sent.Attendees[0].EmailAddress.Address, observed.Attendees[0].EmailAddress.Address)
}

func (m *Module) SetInterviewReminder(ctx context.Context, tx pgx.Tx, p authz.Principal, id string, in api.InterviewOutlookReminderInput) (api.InterviewInvitation, error) {
	s := m.service
	if err := m.LockInterview(ctx, tx, id); err != nil {
		return api.InterviewInvitation{}, err
	}
	meeting, err := s.deps.Sources.Meeting(ctx, tx, "interview", id)
	if err != nil {
		return api.InterviewInvitation{}, err
	}
	if meeting.Version != in.Version {
		return api.InterviewInvitation{}, apperr.StaleVersion("interview")
	}
	row, err := readInterviewInvitation(ctx, tx, p.TenantID, id)
	if err != nil {
		return api.InterviewInvitation{}, err
	}
	if row.ID == "" || row.UserID != p.UserID || !p.Can(api.PermissionCalendarsUse) {
		return api.InterviewInvitation{}, invitationDenied("The original organizer manages the reminder email.")
	}
	if row.CandidateReminder == in.Enabled {
		return m.InterviewInvitation(ctx, tx, p, id)
	}
	q := store.New(tx)
	if in.Enabled {
		if !meeting.Scheduled || !meeting.Start.After(s.now().Add(candidateReminderCutoff)) {
			return api.InterviewInvitation{}, invitationDenied("Choose an interview more than an hour away to enable the email reminder.")
		}
		conn, err := q.CalendarConnection(ctx, p.UserID)
		if database.IsNoRows(err) || err == nil && (!inviteCalendar(row, conn) || conn.Status == "reconnect") {
			return api.InterviewInvitation{}, invitationDenied("Reconnect the original Outlook calendar to enable the email reminder.")
		}
		if err != nil {
			return api.InterviewInvitation{}, err
		}
		if _, issue, err := s.invitationRecipient(ctx, tx, meeting, row); err != nil {
			return api.InterviewInvitation{}, err
		} else if issue != "" {
			return api.InterviewInvitation{}, invitationDenied(issue)
		}
	}
	row, err = q.SetCandidateReminder(ctx, store.SetCandidateReminderParams{ID: row.ID, Enabled: in.Enabled})
	if err != nil {
		return api.InterviewInvitation{}, err
	}
	if int(row.SourceVersion) == meeting.Version {
		if err := s.queueCandidateReminder(ctx, tx, p.TenantID, row, meeting); err != nil {
			return api.InterviewInvitation{}, err
		}
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: "interview.outlook_reminder_changed", EntityType: "interview", EntityID: id, Changes: map[string]any{"enabled": in.Enabled}}); err != nil {
		return api.InterviewInvitation{}, err
	}
	return m.InterviewInvitation(ctx, tx, p, id)
}

func (s *Service) queueCandidateReminder(ctx context.Context, tx pgx.Tx, tenant string, row store.CalendarEvent, meeting Meeting) error {
	if !row.CandidateReminder || !meeting.Scheduled || !meeting.Start.After(s.now().Add(candidateReminderCutoff)) ||
		row.Status != "active" || s.deps.ReminderMail == nil {
		return nil
	}
	email, err := candidateAttendee(row)
	if err != nil || email == "" {
		return err
	}
	n, err := store.New(tx).ReserveCandidateReminder(ctx, store.ReserveCandidateReminderParams{ID: row.ID, ExpectedVersion: row.Version})
	if err != nil || n == 0 {
		return err
	}
	at := meeting.Start.Add(-candidateReminderLead)
	if at.Before(s.now()) {
		at = s.now()
	}
	payload := api.InterviewReminderDelivery{EventId: row.ID, StartsAt: meeting.Start, Version: int(row.CandidateReminderVersion)}
	return s.deps.ReminderMail.QueueInterviewReminder(ctx, tx, tenant, meeting.ApplicationID, email, at, payload)
}

// ReminderBody runs in the email delivery transaction. Its source lock keeps
// the last-minute decision aligned with interview edits and reminder toggles.
func (m *Module) ReminderBody(ctx context.Context, tx pgx.Tx, tenant string, job api.InterviewReminderDelivery, to []string) (string, error) {
	s := m.service
	q := store.New(tx)
	initial, err := q.CalendarEvent(ctx, job.EventId)
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil || initial.Source != invitationSource {
		return "", err
	}
	if err := m.LockInterview(ctx, tx, text(initial.SourceID)); err != nil {
		return "", err
	}
	row, err := q.CalendarEvent(ctx, job.EventId)
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !row.CandidateReminder || int(row.CandidateReminderVersion) != job.Version || row.Status != "active" ||
		!row.StartsAt.Equal(job.StartsAt) || !row.StartsAt.After(s.now().Add(candidateReminderCutoff)) ||
		text(row.CandidateResponse) == "declined" || row.CandidateReminderSentFor != nil && row.CandidateReminderSentFor.Equal(job.StartsAt) {
		return "", nil
	}
	organizer, active, err := s.deps.Users.PrincipalFor(ctx, tx, row.UserID)
	if err != nil || !active || !organizer.Can(api.PermissionInterviewsSchedule) || !organizer.Can(api.PermissionCalendarsUse) {
		return "", err
	}
	var body string
	err = database.AsPrincipal(ctx, tx, organizer, func() error {
		meeting, err := s.deps.Sources.Meeting(ctx, tx, "interview", text(row.SourceID))
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil || !meeting.Scheduled || !meeting.Start.Equal(job.StartsAt) {
			return err
		}
		conn, err := q.CalendarConnection(ctx, row.UserID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil || !inviteCalendar(row, conn) || conn.Status == "reconnect" {
			return err
		}
		if row.SyncStatus == "pending" || int(row.SourceVersion) != meeting.Version && row.SyncStatus == "synced" {
			return jobqueue.Later(s.now().Add(time.Minute))
		}
		if row.SyncStatus != "synced" || row.RemoteID == nil {
			return nil
		}
		email, issue, err := s.invitationRecipient(ctx, tx, meeting, row)
		if err != nil || issue != "" || len(to) != 1 || !strings.EqualFold(email, to[0]) {
			return err
		}
		body = candidateReminderText(meeting, text(row.JoinUrl), s.config.TimeZone)
		return nil
	})
	return body, err
}

func (m *Module) ReminderSent(ctx context.Context, tx pgx.Tx, job api.InterviewReminderDelivery) error {
	return store.New(tx).MarkCandidateReminded(ctx, store.MarkCandidateRemindedParams{
		ID: job.EventId, ReminderVersion: int32(job.Version), StartsAt: &job.StartsAt, //nolint:gosec // matched to the database version before delivery
	})
}

func candidateReminderText(meeting Meeting, join string, zone *time.Location) string {
	body := "A reminder about your upcoming interview.\n\n" + meeting.Title + "\n"
	body += "When: " + humantime.When(meeting.Start, zone) + "\n"
	if join != "" {
		body += "Join in Microsoft Teams: " + join + "\n"
	} else if meeting.Location != "" {
		body += "Where: " + meeting.Location + "\n"
	}
	if meeting.Instructions != "" {
		body += "\n" + meeting.Instructions + "\n"
	}
	return body + "\nSee your Outlook invitation for the full details."
}
