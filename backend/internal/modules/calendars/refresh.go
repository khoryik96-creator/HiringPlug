package calendars

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Refresh replaces a complete window atomically. Partial pages never erase
// known busy times. A sequence makes obsolete or duplicated jobs harmless.
func (s *Service) refresh(ctx context.Context, job jobqueue.Job) error {
	var payload api.CalendarRefreshJob
	if err := job.Decode(&payload); err != nil {
		return err
	}
	initial, err := s.connectionByID(ctx, job.TenantID, payload.ConnectionId)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.network(ctx, job.TenantID, initial.UserID, func(ctx context.Context, conn *pgx.Conn) error {
		p := authz.System(job.TenantID)
		var current store.CalendarConnection
		err := database.InTx(ctx, conn, p, func(t pgx.Tx) error {
			var err error
			current, err = store.New(t).CalendarConnectionByID(ctx, payload.ConnectionId)
			return err
		})
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if int(current.SyncSequence) != payload.Sequence || current.Status == "reconnect" {
			return nil
		}
		from, to := syncWindow(s.now())
		token, err := s.access(ctx, conn, current)
		var events []graphEvent
		if err == nil {
			events, err = s.readEvents(ctx, current, token, from, to)
		}
		// A missing calendarView entry may simply have moved outside the window.
		// Resolve our managed entries by identity before declaring them deleted.
		missing := map[string]bool{}
		if err == nil {
			var previous []store.CalendarEvent
			err = database.InTx(ctx, conn, p, func(t pgx.Tx) error {
				var err error
				previous, err = store.New(t).RemoteEventsInWindow(ctx, store.RemoteEventsInWindowParams{UserID: current.UserID, ConnectionID: ptr(current.ID), FromTime: from, ToTime: to})
				return err
			})
			seen := map[string]bool{}
			for _, e := range events {
				seen[e.ID] = true
			}
			for _, row := range previous {
				if err != nil {
					break
				}
				if row.Source == "outlook" || row.RemoteID == nil || seen[*row.RemoteID] {
					continue
				}
				e, readErr := s.oneRemote(ctx, current, *row.RemoteID, token)
				var provider *providerError
				if errors.As(readErr, &provider) && provider.Status == 404 {
					missing[row.ID] = true
					continue
				}
				if readErr != nil {
					err = readErr
					break
				}
				if _, _, err = e.times(); err != nil {
					break
				}
				events = append(events, e)
			}
		}
		if err != nil {
			return s.refreshFailure(ctx, conn, p, current, err)
		}
		return database.InTx(ctx, conn, p, func(t pgx.Tx) error {
			q := store.New(t)
			if err := q.LockCalendarUser(ctx, current.UserID); err != nil {
				return err
			}
			latest, err := q.CalendarConnectionByID(ctx, current.ID)
			if database.IsNoRows(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if latest.SyncSequence != current.SyncSequence {
				return nil
			}
			if err := s.applySnapshot(ctx, t, current, events, missing, from, to); err != nil {
				return err
			}
			if err := s.reconcileHolds(ctx, t, current.UserID, from); err != nil {
				return err
			}
			changed, err := q.FinishOutlookRefresh(ctx, store.FinishOutlookRefreshParams{ID: current.ID, Sequence: current.SyncSequence})
			if err != nil || changed == 0 {
				return err
			}
			now := s.now()
			if err := q.SaveOutlookAvailability(ctx, store.SaveOutlookAvailabilityParams{UserID: current.UserID, ConnectionID: current.ID, Status: "ok", SyncedAt: &now, WindowStart: &from, WindowEnd: &to}); err != nil {
				return err
			}
			_, err = jobqueue.EnqueueAt(ctx, t, p.TenantID, api.JobKindCalendarRefresh, api.CalendarRefreshJob{ConnectionId: current.ID, Sequence: int(current.SyncSequence) + 1}, now.Add(5*time.Minute))
			return err
		})
	})
}
func syncWindow(now time.Time) (time.Time, time.Time) {
	day := now.UTC().Truncate(24 * time.Hour)
	return day.AddDate(0, 0, -31), day.AddDate(0, 0, 94)
}
func (s *Service) refreshFailure(ctx context.Context, conn *pgx.Conn, p authz.Principal, c store.CalendarConnection, cause error) error {
	return database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		status := "failed"
		if needsReconnect(cause) {
			status = "reconnect"
		}
		q := store.New(tx)
		changed, err := q.FailOutlookRefresh(ctx, store.FailOutlookRefreshParams{ID: c.ID, Sequence: c.SyncSequence, Status: status, SyncError: ptr(cause.Error())})
		if err != nil || changed == 0 {
			return err
		}
		if err := q.SaveOutlookAvailability(ctx, store.SaveOutlookAvailabilityParams{UserID: c.UserID, ConnectionID: c.ID, Status: status}); err != nil {
			return err
		}
		if status == "reconnect" {
			return nil
		}
		delay := 5 * time.Minute
		var provider *providerError
		if errors.As(cause, &provider) && provider.Retry > delay {
			delay = provider.Retry
		}
		_, err = jobqueue.EnqueueAt(ctx, tx, p.TenantID, api.JobKindCalendarRefresh, api.CalendarRefreshJob{ConnectionId: c.ID, Sequence: int(c.SyncSequence) + 1}, s.now().Add(delay))
		return err
	})
}

// reconcileHolds checks the user's live interview and booking holds against
// their sources at each sync: one whose interview was deleted (with its
// candidate, say) is cancelled and taken out of Outlook.
func (s *Service) reconcileHolds(ctx context.Context, tx pgx.Tx, user string, from time.Time) error {
	holds, err := store.New(tx).ActiveSourceHolds(ctx, store.ActiveSourceHoldsParams{UserID: user, FromTime: from})
	if err != nil {
		return err
	}
	keys := make([]sourceKey, 0, len(holds))
	for _, h := range holds {
		if h.SourceID != nil {
			keys = append(keys, sourceKey{h.Source, *h.SourceID})
		}
	}
	_, err = s.gone(ctx, tx, keys)
	return err
}
