package calendars

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

func (s *Service) applySnapshot(ctx context.Context, tx pgx.Tx, c store.CalendarConnection, events []graphEvent, missing map[string]bool, from, to time.Time) error {
	q := store.New(tx)
	old, err := q.RemoteEventsInWindow(ctx, store.RemoteEventsInWindowParams{UserID: c.UserID, ConnectionID: ptr(c.ID), FromTime: from, ToTime: to})
	if err != nil {
		return err
	}
	if err := q.ClearOutlookBusy(ctx, c.UserID); err != nil {
		return err
	}
	busy := store.InsertOutlookBusyParams{UserID: c.UserID}
	seen := map[string]bool{}
	for _, e := range events {
		start, end, err := e.times()
		if err != nil {
			return err
		}
		seen[e.ID] = true
		row, err := q.RemoteCalendarEvent(ctx, store.RemoteCalendarEventParams{UserID: c.UserID, ConnectionID: ptr(c.ID), RemoteID: ptr(e.ID)})
		unknown := database.IsNoRows(err)
		if err != nil && !unknown {
			return err
		}
		source, sourceID := "outlook", e.ID
		if !unknown && row.Source != "outlook" {
			source, sourceID = row.Source, row.ID
			if row.SourceID != nil {
				sourceID = *row.SourceID
			}
			if err := s.mergeRemote(ctx, tx, row, e); err != nil {
				return err
			}
		} else if !e.IsCancelled {
			attendees, _ := json.Marshal(e.attendees())
			title := bounded(e.Subject, 200)
			if title == "" {
				title = "Busy"
			}
			if err := q.ImportOutlookEvent(ctx, store.ImportOutlookEventParams{UserID: c.UserID, Title: title, Notes: bounded(e.BodyPreview, 4000), Location: bounded(e.Location.DisplayName, 500), StartsAt: start, EndsAt: end, AllDay: e.IsAllDay, Attendees: attendees, SourceID: ptr(c.ID + ":" + e.ID), RemoteID: ptr(e.ID), ConnectionID: ptr(c.ID), RemoteKey: e.key(), WebUrl: safeWebURL(e.WebLink), JoinUrl: e.join()}); err != nil {
				return err
			}
		} else if !unknown {
			if err := q.RemoveImportedCalendarEvent(ctx, row.ID); err != nil {
				return err
			}
		}
		if !e.IsCancelled && e.ShowAs != "free" && start.Before(to) && end.After(from) {
			busy.Starts = append(busy.Starts, start)
			busy.Ends = append(busy.Ends, end)
			busy.Sources = append(busy.Sources, source)
			busy.SourceIds = append(busy.SourceIds, sourceID)
		}
	}
	for _, row := range old {
		if row.RemoteID == nil || seen[*row.RemoteID] {
			continue
		}
		if row.Source == "outlook" {
			if err := q.RemoveImportedCalendarEvent(ctx, row.ID); err != nil {
				return err
			}
		} else if missing[row.ID] {
			if row.Source == "local" {
				if err := q.CancelManagedFromOutlook(ctx, row.ID); err != nil {
					return err
				}
				updated, err := q.CalendarEvent(ctx, row.ID)
				if err != nil {
					return err
				}
				if err := managedBusy(ctx, tx, updated); err != nil {
					return err
				}
			} else {
				if err := q.MarkCalendarConflict(ctx, store.MarkCalendarConflictParams{ID: row.ID, RemoteSnapshot: []byte(`{"deleted":true}`)}); err != nil {
					return err
				}
			}
		}
	}
	if len(busy.Starts) > 0 {
		return q.InsertOutlookBusy(ctx, busy)
	}
	return nil
}
func (s *Service) mergeRemote(ctx context.Context, tx pgx.Tx, row store.CalendarEvent, e graphEvent) error {
	if text(row.RemoteKey) == text(e.key()) || row.Version != row.SentVersion {
		return nil
	}
	q := store.New(tx)
	snapshot, _ := json.Marshal(e)
	start, end, err := e.times()
	if err != nil {
		return err
	}
	if row.Source != "local" {
		if e.IsCancelled || !start.Equal(row.StartsAt) || !end.Equal(row.EndsAt) {
			return q.MarkCalendarConflict(ctx, store.MarkCalendarConflictParams{ID: row.ID, RemoteKey: e.key(), RemoteSnapshot: snapshot})
		}
		return nil
	}
	notes := row.Notes
	if e.Body.ContentType == "text" {
		notes = bounded(e.Body.Content, 4000)
	}
	attendees, _ := json.Marshal(e.attendees())
	title := bounded(e.Subject, 200)
	if title == "" {
		title = "Busy"
	}
	status := "active"
	if e.IsCancelled {
		status = "cancelled"
	}
	if err := q.UpdateManagedFromOutlook(ctx, store.UpdateManagedFromOutlookParams{ID: row.ID, Title: title, Notes: notes, Location: bounded(e.Location.DisplayName, 500), StartsAt: start, EndsAt: end, AllDay: e.IsAllDay, Attendees: attendees, Status: status, RemoteKey: e.key(), WebUrl: safeWebURL(e.WebLink), JoinUrl: e.join(), RemoteSnapshot: snapshot}); err != nil {
		return err
	}
	updated, err := q.CalendarEvent(ctx, row.ID)
	if err != nil {
		return err
	}
	return managedBusy(ctx, tx, updated)
}
