package calendars

// Changes the organizer makes in Outlook to a meeting HyPlug sent. An
// interview invitation takes them in, and so does its interview (title, place,
// instructions, time) through the interviews module; nothing is sent back to
// Outlook, which already sent the update. A deleted invitation, or one whose
// candidate was taken off, still waits for review (that would cancel the
// interview). A change is taken only while the organizer may still schedule
// interviews and sees the candidate, and only for an interview still
// scheduled; otherwise a move waits for review, as before. A private
// interview or booking hold keeps a rename, a place or notes (people added
// there stay in Outlook only, so a hold made again never invites them), but a
// hold moved in Outlook waits for review: it is one person's copy, and moving
// it must not move the meeting for the others.

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// mergeManaged handles an Outlook change to an invitation or a hold (any
// source but "local"), once it is known to be new and nothing of HyPlug's
// is on its way to Outlook.
func (s *Service) mergeManaged(ctx context.Context, tx pgx.Tx, tenant string, row store.CalendarEvent, e graphEvent, missingAttendee bool) error {
	q := store.New(tx)
	snapshot, _ := json.Marshal(e)
	start, end, err := e.times()
	if err != nil {
		return err
	}
	conflict := func() error {
		return q.MarkCalendarConflict(ctx, store.MarkCalendarConflictParams{ID: row.ID, RemoteKey: e.key(), RemoteSnapshot: snapshot})
	}
	if missingAttendee || e.IsCancelled {
		return conflict()
	}
	known := knownEvent(row)
	diff := diffOutlook(row, known, e, start, end)
	if teamsArrived(row, known, e) {
		// Outlook filled in the Teams meeting HyPlug asked for: the join
		// details it added to the message, and its Teams place, are no
		// organizer change (what the organizer changed besides is).
		diff = withoutTeamsArrival(diff, *known, e)
	}
	if row.Source == invitationSource && !diff.Time && responseOnly(row, e) {
		return q.AcceptInvitationResponseReceipt(ctx, store.AcceptInvitationResponseReceiptParams{
			ID: row.ID, ExpectedVersion: row.Version, RemoteKey: e.key(), RemoteSnapshot: snapshot,
		})
	}
	taking := row.SyncStatus == "synced" && (row.Source == "interview" || row.Source == "booking" ||
		row.Source == invitationSource && s.deps.Interviews != nil)
	if !taking {
		// Already waiting for review (or nothing here can take it): as before.
		if diff.Time {
			return conflict()
		}
		return nil
	}
	if diff.empty() {
		return q.AcceptInvitationResponseReceipt(ctx, store.AcceptInvitationResponseReceiptParams{
			ID: row.ID, ExpectedVersion: row.Version, RemoteKey: e.key(), RemoteSnapshot: snapshot,
		})
	}
	if row.Source != invitationSource {
		if diff.Time {
			return conflict()
		}
		return s.takeHoldChange(ctx, tx, row, e, snapshot, diff)
	}
	if diff.Time && e.IsAllDay {
		return conflict() // an interview is never all day
	}
	return s.takeInvitationChange(ctx, tx, tenant, row, e, snapshot, known, diff, conflict)
}

// takeHoldChange keeps on a hold what was changed in Outlook other than its
// time. A later change of its interview or booking sets its title and place
// again, as the meeting has them. People added in Outlook stay in Outlook
// (and in the receipt) but never on the hold: a hold made again (in another
// account, after a reconnect) would invite them.
func (s *Service) takeHoldChange(ctx context.Context, tx pgx.Tx, row store.CalendarEvent, e graphEvent, snapshot []byte, diff outlookDiff) error {
	notes := row.Notes
	if diff.Body && e.Body.ContentType == "text" {
		notes = bounded(strings.TrimSpace(e.Body.Content), 4000)
	}
	title := bounded(strings.TrimSpace(e.Subject), 200)
	if title == "" {
		title = row.Title
	}
	n, err := store.New(tx).AcceptOutlookHoldChange(ctx, store.AcceptOutlookHoldChangeParams{
		ID: row.ID, ExpectedVersion: row.Version, Title: title, Notes: notes, Location: bounded(strings.TrimSpace(e.Location.DisplayName), 500),
		RemoteKey: e.key(), RemoteSnapshot: snapshot, WebUrl: safeWebURL(e.WebLink),
	})
	if err != nil || n == 0 {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "calendar.outlook_hold_changed", EntityType: row.Source, EntityID: text(row.SourceID),
		Changes: map[string]any{"fields": diff.fields()}})
}

// takeInvitationChange takes an Outlook change into an interview invitation
// and its interview, as its organizer's: only while they may still schedule
// interviews and see this one (its candidate in their scope, not binned), and
// only into a scheduled interview. Otherwise it is as before: a move waits for
// review, anything else is left. It never waits for the interview's lock: a
// HyPlug edit or a calendar job holding it may be waiting for this
// invitation's row, which this refresh may already have written. A busy
// interview, or one with a HyPlug change not yet sent, is left for the
// next refresh (when both sides changed, that change meets Outlook's and is
// flagged for review).
func (s *Service) takeInvitationChange(ctx context.Context, tx pgx.Tx, tenant string, row store.CalendarEvent, e graphEvent, snapshot []byte, known *graphEvent, diff outlookDiff, conflict func() error) error {
	q := store.New(tx)
	id := text(row.SourceID)
	people := e.attendees()
	if len(people) > maxEventAttendees {
		return conflict() // more people than an invitation holds
	}
	notTaken := func() error {
		if diff.Time {
			return conflict()
		}
		return nil
	}
	if s.deps.Users == nil {
		return notTaken()
	}
	organizer, active, err := s.deps.Users.PrincipalFor(ctx, tx, row.UserID)
	if err != nil {
		return err
	}
	if !active || !organizer.Can(api.PermissionInterviewsSchedule) || !organizer.Can(api.PermissionCalendarsUse) {
		return notTaken()
	}
	if free, err := q.TryLockCalendarSource(ctx, store.TryLockCalendarSourceParams{Kind: "interview", SourceID: id}); err != nil || !free {
		return err
	}
	// Read again under the lock: a calendar job may have changed it since.
	current, err := q.CalendarEvent(ctx, row.ID)
	if err != nil {
		return err
	}
	if current.Version != row.Version || current.Version != current.SentVersion || current.SyncStatus != "synced" || text(current.RemoteKey) != text(row.RemoteKey) {
		return nil
	}
	// Read as the organizer: an interview they no longer see is not theirs to change.
	var meeting Meeting
	err = database.AsPrincipal(ctx, tx, organizer, func() error {
		var err error
		meeting, err = s.deps.Sources.Meeting(ctx, tx, "interview", id)
		return err
	})
	if database.IsNoRows(err) {
		return notTaken() // deleted (its invitation is cancelled by the calendar job), binned or out of their scope
	}
	if err != nil {
		return err
	}
	if !meeting.Scheduled {
		return notTaken() // completed, no-show or cancelled: HyPlug no longer schedules it
	}
	if meeting.Version != int(row.SourceVersion) {
		return nil // a HyPlug change not projected yet
	}
	body := strings.TrimSpace(e.Body.Content)
	// Only a plain invitation's message is its instructions: a formatted one
	// (or one already rewritten in Outlook, which no longer keeps its
	// formatting) is a whole message, and a Teams one carries the join details.
	instructions := diff.Body && e.Body.ContentType == "text" && row.NotesHtml == nil && !row.OutlookBody && !row.Teams &&
		(known == nil || known.OnlineMeeting == nil) && e.OnlineMeeting == nil && utf8.RuneCountInString(body) <= 2000
	var change MeetingChange
	if diff.Title {
		change.Title = &e.Subject
	}
	if diff.Location {
		location := strings.TrimSpace(e.Location.DisplayName)
		change.Location = &location
	}
	if instructions {
		change.Instructions = &body
	}
	if diff.Time {
		start, end, _ := e.times() // checked by the caller
		change.Start, change.End = &start, &end
	}
	after, refused := meeting, false
	if change != (MeetingChange{}) {
		sp, err := tx.Begin(ctx) // a savepoint: a refusal never undoes the refresh
		if err != nil {
			return err
		}
		changed, err := s.deps.Interviews.ChangeFromOutlook(ctx, sp, tenant, id, row.UserID, meeting.Version, change)
		if problem, ok := apperr.As(err); ok {
			if err := sp.Rollback(ctx); err != nil {
				return err
			}
			if problem.Code == api.ErrorCodeCONFLICTSTALEVERSION {
				return nil
			}
			if diff.Time {
				return conflict() // a time HyPlug does not schedule (not scheduled any more, too long)
			}
			refused = true
		} else if err != nil {
			_ = sp.Rollback(ctx)
			return err
		} else if err := sp.Commit(ctx); err != nil {
			return err
		} else {
			after = changed
		}
	}
	// The interview's values as it now holds them, so its calendar job finds
	// nothing to send; what the interview does not hold, as Outlook has it.
	title, location, start, end := after.Title, bounded(after.Location, 500), after.Start, after.End
	if refused {
		title, location, start, end = row.Title, row.Location, row.StartsAt, row.EndsAt
		if subject := strings.TrimSpace(e.Subject); diff.Title && subject != "" {
			title = bounded(subject, 100)
		}
		if diff.Location {
			location = bounded(strings.TrimSpace(e.Location.DisplayName), 500)
		}
	}
	notes, rewritten := row.Notes, false
	if diff.Body && e.Body.ContentType == "text" {
		if instructions && !refused {
			notes = after.Instructions
		} else {
			notes, rewritten = bounded(body, 4000), true
		}
	}
	attendees, _ := json.Marshal(people)
	updated, err := q.ApplyOutlookInvitationChange(ctx, store.ApplyOutlookInvitationChangeParams{
		ID: row.ID, ExpectedVersion: row.Version, Title: title, Notes: notes, OutlookBody: rewritten, Location: location,
		StartsAt: start, EndsAt: end, Attendees: attendees, Online: e.OnlineMeeting != nil, JoinUrl: e.join(), WebUrl: safeWebURL(e.WebLink),
		// The Teams meeting turned off in Outlook: a later change must not ask for a new one.
		TeamsOff:  known != nil && known.OnlineMeeting != nil && e.OnlineMeeting == nil,
		RemoteKey: e.key(), RemoteSnapshot: snapshot, SourceVersion: int32(after.Version), //nolint:gosec // source versions are int32
	})
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := managedBusy(ctx, tx, updated); err != nil {
		return err
	}
	if after.Version != meeting.Version {
		if !after.Start.Equal(meeting.Start) {
			if err := s.queueCandidateReminder(ctx, tx, tenant, updated, after); err != nil {
				return err
			}
		}
		// The rest of the panel's holds follow the interview.
		if _, err := jobqueue.Enqueue(ctx, tx, tenant, api.JobKindCalendarSourceChanged, api.CalendarSourceJob{Kind: "interview", SourceId: id}); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "interview.outlook_invitation_changed", EntityType: "interview", EntityID: id,
		Changes: map[string]any{"fields": diff.fields(), "attendees": len(people), "interviewChanged": after.Version != meeting.Version, "by": row.UserID}})
}
