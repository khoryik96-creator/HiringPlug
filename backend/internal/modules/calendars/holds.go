package calendars

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
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
	m, err := s.deps.Sources.Meeting(ctx, tx, string(payload.Kind), payload.SourceId)
	if database.IsNoRows(err) {
		m = Meeting{Kind: string(payload.Kind), ID: payload.SourceId, Cancelled: true}
	} else if err != nil {
		return err
	}
	q := store.New(tx)
	previous, err := q.SourceCalendarEvents(ctx, store.SourceCalendarEventsParams{Source: string(payload.Kind), SourceID: ptr(payload.SourceId)})
	if err != nil {
		return err
	}
	for _, old := range previous {
		if !slices.Contains(m.Users, old.UserID) || m.Cancelled {
			row, err := q.CancelCalendarHold(ctx, old.ID)
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
				if err := s.queuePush(ctx, tx, job.TenantID, row.ID, conn.ID); err != nil {
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
		active, err := s.deps.Users.CalendarUserActive(ctx, tx, user)
		if err != nil {
			return err
		}
		if !active {
			continue
		}
		if _, err := s.holdIn(ctx, tx, job.TenantID, user, m); err != nil {
			return err
		}
	}
	return nil
}

// Busy reads only minimal availability rows. Missing/stale provider data is
// unknown, not free; disconnected users retain the original local behavior.
func (s *Service) busy(ctx context.Context, tx pgx.Tx, users []string, from, to time.Time, exceptKind, exceptID string) ([]ical.Interval, error) {
	q := store.New(tx)
	health, err := q.OutlookHealthForUsers(ctx, users)
	if err != nil {
		return nil, err
	}
	for _, h := range health {
		if h.Status != "ok" || h.SyncedAt == nil || s.now().Sub(*h.SyncedAt) > 15*time.Minute || h.WindowStart == nil || h.WindowEnd == nil || from.Before(*h.WindowStart) || to.After(*h.WindowEnd) {
			return nil, apperr.New(api.ErrorCodeCALENDARUNAVAILABLE, "Outlook availability is not up to date. Refresh the connected calendar before scheduling.")
		}
	}
	rows, err := q.OutlookBusyForUsers(ctx, store.OutlookBusyForUsersParams{UserIds: users, FromTime: from, ToTime: to, ExceptKind: exceptKind, ExceptID: exceptID})
	if err != nil {
		return nil, err
	}
	out := make([]ical.Interval, 0, len(rows))
	for _, r := range rows {
		out = append(out, ical.Interval{Start: r.StartsAt, End: r.EndsAt})
	}
	return out, nil
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
		return nil
	}
	return q.SaveManagedBusy(ctx, store.SaveManagedBusyParams{UserID: row.UserID, Source: row.Source, SourceID: sourceID, StartsAt: row.StartsAt, EndsAt: row.EndsAt})
}
