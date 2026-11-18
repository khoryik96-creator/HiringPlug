package calendars

import (
	"context"
	"encoding/json"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	c "github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const invitationSource = "interview_invitation"

func (m *Module) LockInterview(ctx context.Context, tx pgx.Tx, id string) error {
	return store.New(tx).LockCalendarSource(ctx, store.LockCalendarSourceParams{Kind: "interview", SourceID: id})
}

func (m *Module) InterviewJoinLink(ctx context.Context, tx pgx.Tx, tenant, id string) (string, error) {
	row, err := readInterviewInvitation(ctx, tx, tenant, id)
	if err != nil || row.Status != "active" || row.RemoteID == nil {
		return "", err
	}
	return text(row.JoinUrl), nil
}

// InterviewInvitation exposes only the source interview's invite state and
// join link. The interviews module verifies access to that interview first.
func (m *Module) InterviewInvitation(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (api.InterviewInvitation, error) {
	s := m.service
	row, err := readInterviewInvitation(ctx, tx, p.TenantID, id)
	out := invitationAPI(row)
	if err != nil {
		return out, err
	}
	meeting, err := s.deps.Sources.Meeting(ctx, tx, "interview", id)
	if err != nil {
		return out, err
	}
	if meeting.Cancelled {
		out.JoinUrl = c.OrNull[string](nil)
	}
	if !p.Can(api.PermissionInterviewsSchedule) || !p.Can(api.PermissionCalendarsUse) || row.ID != "" && row.UserID != p.UserID {
		return out, nil
	}
	conn, err := store.New(tx).CalendarConnection(ctx, p.UserID)
	if err != nil && !database.IsNoRows(err) {
		return out, err
	}
	if conn.ID == "" || conn.Status == "reconnect" || row.ID != "" && !inviteCalendar(row, conn) {
		if row.ID == "" {
			out.Issue = c.OrNull(ptr("Connect Outlook Calendar to send a native invitation."))
		} else if out.Status != "cancelled" {
			out.Issue = c.OrNull(ptr("Reconnect the original Outlook calendar to manage this invitation."))
		}
		return out, nil
	}
	out.TeamsAvailable = conn.TeamsAvailable && meeting.Mode == "video"
	if row.ID == "" && (!meeting.Scheduled || !meeting.Start.After(s.now())) {
		return out, nil
	}
	if row.ID != "" && out.Status != "failed" && out.Status != "conflict" {
		return out, nil
	}
	recipient, issue, err := s.invitationRecipient(ctx, tx, meeting, row)
	if err != nil {
		return out, err
	}
	if issue != "" {
		out.Issue = c.OrNull(ptr(issue))
		return out, nil
	}
	out.Recipient = c.OrNull(ptr(recipient))
	out.CanSend = s.canManageInvitation(meeting, row)
	return out, nil
}

func (m *Module) SendInterviewInvitation(ctx context.Context, tx pgx.Tx, p authz.Principal, id string, in api.InterviewInvitationInput) (api.InterviewInvitation, error) {
	s := m.service
	if !p.Can(api.PermissionCalendarsUse) {
		return api.InterviewInvitation{}, apperr.New(api.ErrorCodeFORBIDDEN, "Your role cannot use Outlook Calendar.")
	}
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
	out := invitationAPI(row)
	if err != nil {
		return out, err
	}
	if row.ID != "" && row.UserID != p.UserID {
		return out, invitationDenied("The original organizer manages this invitation.")
	}
	q := store.New(tx)
	conn, err := q.CalendarConnection(ctx, p.UserID)
	if database.IsNoRows(err) || err == nil && conn.Status == "reconnect" {
		return out, invitationDenied("Connect Outlook Calendar to send the invitation.")
	}
	if err != nil {
		return out, err
	}
	if row.ID != "" {
		if !inviteCalendar(row, conn) {
			return out, invitationDenied("Reconnect the original Outlook calendar to manage this invitation.")
		}
		if row.SyncStatus != "failed" && row.SyncStatus != "conflict" {
			return out, nil
		}
	}
	if !s.canManageInvitation(meeting, row) {
		return out, invitationDenied("Choose a future scheduled interview to send an invitation.")
	}
	email, issue, err := s.invitationRecipient(ctx, tx, meeting, row)
	if err != nil {
		return out, err
	}
	if issue != "" {
		return out, invitationDenied(issue)
	}
	if row.ID == "" {
		if in.Teams && (!conn.TeamsAvailable || meeting.Mode != "video") {
			return out, apperr.Validation(apperr.FieldErrors{{Field: "teams", Message: "choose a video interview and a Teams-enabled Outlook calendar"}})
		}
		attendees, _ := json.Marshal([]api.CalendarAttendee{{Email: toEmail(email)}})
		row, err = q.InsertInterviewInvitation(ctx, store.InsertInterviewInvitationParams{
			InterviewID: ptr(id), SourceVersion: int32(in.Version), Title: meeting.Title, Notes: meeting.Instructions, Location: bounded(meeting.Location, 500), //nolint:gosec // checked against the database version
			StartsAt: meeting.Start, EndsAt: meeting.End, Teams: in.Teams, Attendees: attendees, ReminderMinutes: int32(in.ReminderMinutes), //nolint:gosec // reminder range is bounded by the contract
			InvitationSubject: ptr(conn.SubjectID), InvitationCalendar: ptr(conn.CalendarID),
		})
	} else {
		row, err = s.syncInvitationProjection(ctx, tx, row, meeting)
		if err == nil {
			row, err = q.RetryCalendarEvent(ctx, store.RetryCalendarEventParams{ID: row.ID, ExpectedVersion: row.Version})
		}
	}
	if err != nil {
		return out, err
	}
	if err := managedBusy(ctx, tx, row); err != nil {
		return out, err
	}
	if err := s.queuePush(ctx, tx, p.TenantID, row.ID, conn.ID); err != nil {
		return out, err
	}
	return invitationAPI(row), audit.Record(ctx, tx, audit.Entry{Action: "interview.outlook_invitation_queued", EntityType: "interview", EntityID: id})
}

func (s *Service) canManageInvitation(meeting Meeting, row store.CalendarEvent) bool {
	return meeting.Scheduled && meeting.Start.After(s.now()) || meeting.Cancelled && row.RemoteID != nil
}

func invitationDenied(message string) error {
	return apperr.New(api.ErrorCodeINTERVIEWNOTALLOWED, message)
}

func readInterviewInvitation(ctx context.Context, tx pgx.Tx, tenant, id string) (store.CalendarEvent, error) {
	var row store.CalendarEvent
	err := database.AsSystem(ctx, tx, tenant, func() error {
		var err error
		row, err = store.New(tx).InterviewInvitation(ctx, ptr(id))
		if database.IsNoRows(err) {
			return nil
		}
		return err
	})
	return row, err
}

func invitationAPI(row store.CalendarEvent) api.InterviewInvitation {
	status := "none"
	if row.ID != "" {
		status = row.SyncStatus
		if status == "synced" {
			status = "sent"
			if row.Status == "cancelled" {
				status = "cancelled"
			}
		}
	}
	link := row.JoinUrl
	if row.Status != "active" {
		link = nil
	}
	return api.InterviewInvitation{Status: api.InterviewInvitationStatus(status), Teams: row.Teams, ReminderMinutes: int(row.ReminderMinutes),
		Recipient: c.OrNull[string](nil), JoinUrl: c.OrNull(link), Issue: c.OrNull(row.SyncError)}
}

func inviteCalendar(row store.CalendarEvent, conn store.CalendarConnection) bool {
	return conn.ID != "" && text(row.InvitationSubject) == conn.SubjectID && text(row.InvitationCalendar) == conn.CalendarID
}

func (s *Service) invitationRecipient(ctx context.Context, tx pgx.Tx, meeting Meeting, row store.CalendarEvent) (email, issue string, err error) {
	address, declined, err := s.deps.Recipients.InterviewRecipient(ctx, tx, meeting.ApplicationID)
	if err != nil {
		return "", "", err
	}
	if declined {
		return "", "The candidate opted out of being contacted.", nil
	}
	address = strings.TrimSpace(address)
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address || len(address) > 254 {
		return "", "Add a valid email address to the candidate profile before sending.", nil //nolint:nilerr // Invalid candidate email is a user-facing availability reason.
	}
	if row.ID != "" {
		var people []api.CalendarAttendee
		if err := json.Unmarshal(row.Attendees, &people); err != nil {
			return "", "", err
		}
		if len(people) != 1 || !strings.EqualFold(string(people[0].Email), address) {
			return "", "The candidate email changed. Review the original invitation in Outlook.", nil
		}
	}
	return address, "", nil
}
