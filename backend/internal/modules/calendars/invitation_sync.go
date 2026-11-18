package calendars

import (
	"context"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) syncInvitationProjection(ctx context.Context, tx pgx.Tx, row store.CalendarEvent, meeting Meeting) (store.CalendarEvent, error) {
	if int(row.SourceVersion) >= meeting.Version {
		return row, nil
	}
	status := "active"
	if meeting.Cancelled {
		status = "cancelled"
	}
	title, notes, location, start, end := meeting.Title, meeting.Instructions, meeting.Location, meeting.Start, meeting.End
	if meeting.Version == math.MaxInt32 {
		// The interview was deleted. Keep the receipt, stop exposing its busy
		// interval, and never send mail without a live candidate to check.
		title, notes, location, start, end = row.Title, row.Notes, row.Location, row.StartsAt, row.EndsAt
	}
	location = bounded(location, 500)
	if title == row.Title && notes == row.Notes && location == row.Location && start.Equal(row.StartsAt) && end.Equal(row.EndsAt) && status == row.Status {
		return store.New(tx).AdvanceInterviewInvitationSource(ctx, store.AdvanceInterviewInvitationSourceParams{
			ID: row.ID, SourceVersion: int32(meeting.Version), //nolint:gosec // source versions are int32
		})
	}
	return store.New(tx).SyncInterviewInvitation(ctx, store.SyncInterviewInvitationParams{
		ID: row.ID, Title: title, Notes: notes, Location: location, StartsAt: start, EndsAt: end,
		Status: status, SourceVersion: int32(meeting.Version), //nolint:gosec // source versions are int32
	})
}

// reconcileInvitation also names the organizer whose native event replaces
// their private hold. Other panel members keep their private holds.
func (s *Service) reconcileInvitation(ctx context.Context, tx pgx.Tx, tenant string, meeting Meeting) (string, error) {
	q := store.New(tx)
	row, err := q.InterviewInvitation(ctx, ptr(meeting.ID))
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	before := row.Version
	if meeting.Scheduled || meeting.Cancelled {
		row, err = s.syncInvitationProjection(ctx, tx, row, meeting)
		if err != nil {
			return "", err
		}
	}
	if row.Version != before {
		if err := managedBusy(ctx, tx, row); err != nil {
			return "", err
		}
	}
	conn, err := q.CalendarConnection(ctx, row.UserID)
	if err != nil && !database.IsNoRows(err) {
		return "", err
	}
	err = nil // A missing connection is handled as a paused invitation.
	same := inviteCalendar(row, conn)
	if row.SyncStatus == "pending" && row.Version != before {
		switch {
		case meeting.Version == math.MaxInt32:
			err = q.CalendarPushFailed(ctx, store.CalendarPushFailedParams{ID: row.ID, ExpectedVersion: row.Version, SyncError: ptr("This interview was deleted. Manage the original invitation in Outlook.")})
		case same && conn.Status != "reconnect":
			err = s.queuePush(ctx, tx, tenant, row.ID, conn.ID)
		default:
			err = q.CalendarPushFailed(ctx, store.CalendarPushFailedParams{ID: row.ID, ExpectedVersion: row.Version, SyncError: ptr("Reconnect the original Outlook calendar to manage this invitation.")})
		}
	}
	if same && row.Status == "active" && row.RemoteID != nil {
		return row.UserID, err
	}
	return "", err
}

func (s *Service) validateInvitationPush(ctx context.Context, tx pgx.Tx, row store.CalendarEvent, c store.CalendarConnection) error {
	if !inviteCalendar(row, c) {
		return invitationDenied("Reconnect the original Outlook calendar to manage this invitation.")
	}
	meeting, err := s.deps.Sources.Meeting(ctx, tx, "interview", text(row.SourceID))
	if database.IsNoRows(err) {
		return invitationDenied("This interview is unavailable to its original organizer. Manage the invitation in Outlook.")
	}
	if err != nil {
		return err
	}
	if !meeting.Cancelled && !meeting.Scheduled {
		return invitationDenied("This interview is no longer scheduled.")
	}
	if meeting.Version != int(row.SourceVersion) {
		return jobqueue.Later(s.now().Add(5 * time.Second))
	}
	if row.Status == "active" && row.RemoteID == nil && !meeting.Start.After(s.now()) {
		return invitationDenied("This interview has already started.")
	}
	_, issue, err := s.invitationRecipient(ctx, tx, meeting, row)
	if err != nil {
		return err
	}
	if issue != "" {
		return invitationDenied(issue)
	}
	return nil
}

func releaseInvitationLock(conn *pgx.Conn, args store.TryLockInterviewInvitationParams) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := store.New(conn).UnlockInterviewInvitation(ctx, store.UnlockInterviewInvitationParams(args)); err != nil {
		_ = conn.Close(ctx)
	}
}

func sourceOf(kind, id string) sourceKey {
	if kind == invitationSource {
		kind = "interview"
	}
	return sourceKey{kind, id}
}
