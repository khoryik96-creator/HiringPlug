package calendars

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ical"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) holdIn(ctx context.Context, tx pgx.Tx, tenant, user string, m Meeting) (string, error) {
	if !m.End.After(m.Start) {
		return "", nil
	}
	q := store.New(tx)
	conn, err := q.CalendarConnection(ctx, user)
	connected := err == nil
	if err != nil && !database.IsNoRows(err) {
		return "", err
	}
	if connected && m.Kind == "interview" && !m.Cancelled {
		invitation, err := q.InterviewInvitation(ctx, ptr(m.ID))
		if err != nil && !database.IsNoRows(err) {
			return "", err
		}
		if invitation.UserID == user && invitation.RemoteID != nil && invitation.Status == "active" && inviteCalendar(invitation, conn) {
			return "", nil
		}
	}
	status, sync := "active", "local"
	if m.Cancelled {
		status = "cancelled"
	}
	if connected {
		sync = "pending"
	}
	row, err := q.UpsertCalendarHold(ctx, store.UpsertCalendarHoldParams{UserID: user, Title: bounded(m.Title, 200), Location: bounded(m.Location, 500), StartsAt: m.Start, EndsAt: m.End, Source: m.Kind, SourceID: ptr(m.ID), SourceVersion: int32(m.Version), Status: status, SyncStatus: sync}) //nolint:gosec // source database versions are int32
	if database.IsNoRows(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := managedBusy(ctx, tx, row); err != nil {
		return "", err
	}
	if connected {
		if err := s.queuePush(ctx, tx, tenant, row.ID, conn.ID); err != nil {
			return "", err
		}
	}
	return row.ID, nil
}
func (s *Service) sourceChanged(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var payload api.CalendarSourceJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	return s.reconcileSource(ctx, tx, job.TenantID, string(payload.Kind), payload.SourceId)
}

// reconcileSource brings an interview's or booking's holds in line with it as
// it is now (in a transaction scoped to the whole company): made, moved,
// cancelled, or taken off for people no longer on it.
func (s *Service) reconcileSource(ctx context.Context, tx pgx.Tx, tenant, kind, id string) error {
	q := store.New(tx)
	// One at a time per source, each reading it as it is now: a job that read
	// an older version can never finish after a newer one.
	if err := q.LockCalendarSource(ctx, store.LockCalendarSourceParams{Kind: kind, SourceID: id}); err != nil {
		return err
	}
	m, err := s.deps.Sources.Meeting(ctx, tx, kind, id)
	if database.IsNoRows(err) {
		m = Meeting{Kind: kind, ID: id, Cancelled: true, Version: math.MaxInt32} // gone: newer than any hold
	} else if err != nil {
		return err
	}
	previous, err := q.SourceCalendarEvents(ctx, store.SourceCalendarEventsParams{Source: kind, SourceID: ptr(id)})
	if err != nil {
		return err
	}
	organizer := ""
	if kind == "interview" {
		organizer, err = s.reconcileInvitation(ctx, tx, tenant, m)
		if err != nil {
			return err
		}
	}
	for _, old := range previous {
		if !slices.Contains(m.Users, old.UserID) || m.Cancelled || old.UserID == organizer {
			row, err := q.CancelCalendarHold(ctx, store.CancelCalendarHoldParams{ID: old.ID, SourceVersion: int32(m.Version)}) //nolint:gosec // source database versions are int32
			if database.IsNoRows(err) {
				continue
			}
			if err != nil {
				return err
			}
			if err := managedBusy(ctx, tx, row); err != nil {
				return err
			}
			if conn, err := q.CalendarConnection(ctx, old.UserID); err == nil {
				if err := s.queuePush(ctx, tx, tenant, row.ID, conn.ID); err != nil {
					return err
				}
			} else if !database.IsNoRows(err) {
				return err
			}
		}
	}
	if m.Cancelled {
		return nil
	}
	for _, user := range m.Users {
		if user == organizer {
			continue
		}
		active, err := s.deps.Users.CalendarUserActive(ctx, tx, user)
		if err != nil {
			return err
		}
		if !active {
			continue
		}
		if _, err := s.holdIn(ctx, tx, tenant, user, m); err != nil {
			return err
		}
	}
	return nil
}

// busyRows are when users are busy by their calendars: what Outlook said at
// its last sync, kept while a sync is late or failing (a broken connection
// never blocks booking or scheduling; Conflicts reports it instead), and
// HiringPlug's own appointments. A hold whose interview or booking is gone
// (deleted with its candidate, say) is ignored and cleared.
func (s *Service) busyRows(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptKind, exceptID string) ([]store.CalendarBusyRowsRow, error) {
	q := store.New(tx)
	rows, err := q.CalendarBusyRows(ctx, store.CalendarBusyRowsParams{UserIds: users, FromTime: from, ToTime: to, ExceptKind: exceptKind, ExceptID: exceptID})
	if err != nil || s.deps.Sources == nil {
		return rows, err
	}
	keys := make([]sourceKey, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, sourceOf(r.Source, r.SourceID))
	}
	gone, err := s.gone(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, r := range rows {
		if !gone[sourceOf(r.Source, r.SourceID)] {
			out = append(out, r)
		}
	}
	return out, nil
}

type sourceKey struct{ kind, id string }

// gone are the interview and booking holds among keys whose source no longer
// stands (deleted with its candidate, cancelled, ...). They are checked for
// the whole company, not as the caller (an interview on a candidate the
// caller may not see still stands), and cleared at once in tx: cancelled,
// their time freed and their Outlook copies queued for deletion. Nothing is
// queued per read, so a page viewed again and again adds no work.
func (s *Service) gone(ctx context.Context, tx pgx.Tx, keys []sourceKey) (map[sourceKey]bool, error) {
	gone := map[sourceKey]bool{}
	ids := map[string][]string{}
	for _, k := range keys {
		if (k.kind == "interview" || k.kind == "booking") && !slices.Contains(ids[k.kind], k.id) {
			ids[k.kind] = append(ids[k.kind], k.id)
		}
	}
	if s.deps.Sources == nil || len(ids) == 0 {
		return gone, nil
	}
	tenant, err := store.New(tx).CurrentCalendarTenant(ctx)
	if err != nil {
		return nil, err
	}
	err = database.AsSystem(ctx, tx, tenant, func() error {
		for kind, list := range ids {
			live, err := s.deps.Sources.Existing(ctx, tx, kind, list)
			if err != nil {
				return err
			}
			for _, id := range list {
				if !live[id] {
					gone[sourceKey{kind, id}] = true
				}
			}
		}
		for k := range gone {
			if err := s.reconcileSource(ctx, tx, tenant, k.kind, k.id); err != nil {
				return err
			}
		}
		return nil
	})
	return gone, err
}

func (s *Service) busy(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptKind, exceptID string) ([]ical.Interval, error) {
	rows, err := s.busyRows(ctx, tx, users, from, to, exceptKind, exceptID)
	if err != nil {
		return nil, err
	}
	out := make([]ical.Interval, 0, len(rows))
	for _, r := range rows {
		out = append(out, ical.Interval{Start: r.StartsAt, End: r.EndsAt})
	}
	return out, nil
}

// conflicts are, for an interview being scheduled, the users whose calendars
// are busy then (interviews the clash check already named are left out; one
// the scheduler may not see, on another recruiter's candidate, counts as a
// calendar appointment, named by its interviewer only) and the users whose
// Outlook has stopped syncing; all can be scheduled over.
func (s *Service) conflicts(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptID string, named []string) (busy, stale []string, err error) {
	rows, err := s.busyRows(ctx, tx, users, from, to, "interview", exceptID)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		if (sourceOf(r.Source, r.SourceID).kind != "interview" || !slices.Contains(named, r.SourceID)) && !slices.Contains(busy, r.UserID) {
			busy = append(busy, r.UserID)
		}
	}
	health, err := store.New(tx).OutlookHealthForUsers(ctx, users)
	if err != nil {
		return nil, nil, err
	}
	for _, h := range health {
		// Just connected (its first sync on its way) is not "stopped syncing".
		if h.Status != "pending" && !current(h, s.now(), from, to) {
			stale = append(stale, h.UserID)
		}
	}
	return busy, stale, nil
}

// current is whether Outlook's busy times for a user can be trusted for
// from-to: synced lately, over a window that covers it.
func current(h store.OutlookAvailability, now, from, to time.Time) bool {
	return h.Status == "ok" && h.SyncedAt != nil && now.Sub(*h.SyncedAt) <= 15*time.Minute &&
		h.WindowStart != nil && h.WindowEnd != nil && !from.Before(*h.WindowStart) && !to.After(*h.WindowEnd)
}

func managedBusy(ctx context.Context, tx pgx.Tx, row store.CalendarEvent) error {
	q := store.New(tx)
	sourceID := row.ID
	if row.SourceID != nil {
		sourceID = *row.SourceID
	}
	if err := q.ClearManagedBusy(ctx, store.ClearManagedBusyParams{UserID: row.UserID, Source: row.Source, SourceID: sourceID}); err != nil {
		return err
	}
	if row.Status != "active" {
		return q.ClearSourceBusy(ctx, store.ClearSourceBusyParams{UserID: row.UserID, Source: row.Source, SourceID: sourceID})
	}
	if row.Source == "local" && row.SyncStatus == "synced" && row.Version == row.SentVersion && row.RemoteID != nil {
		return nil // in Outlook as it is: Outlook's busy time (or Free) governs
	}
	return q.SaveManagedBusy(ctx, store.SaveManagedBusyParams{UserID: row.UserID, Source: row.Source, SourceID: sourceID, StartsAt: row.StartsAt, EndsAt: row.EndsAt})
}
