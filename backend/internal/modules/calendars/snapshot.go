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
	var ours map[string]store.CalendarEvent // loaded only when an unknown event carries a transaction ID
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
		if e.TransactionID != "" && (unknown || row.Source == "outlook") && ours == nil {
			if ours, err = s.byTransaction(ctx, q, c); err != nil {
				return err
			}
		}
		if mine, ok := ours[e.TransactionID]; ok && e.TransactionID != "" && (unknown || row.Source == "outlook") {
			// Our create reached Outlook but its answer was lost: the event is
			// this row's. Take it back from the import and push the row as it
			// is now (a change, or the delete of a cancelled appointment).
			if row, err = s.adopt(ctx, tx, c, mine, e); err != nil {
				return err
			}
			unknown = false
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
			if row.Source == "local" {
				// Outlook has this appointment: its busy time (or Free) governs
				// from now on, not the interval kept while it was on its way.
				if err := q.ClearManagedBusy(ctx, store.ClearManagedBusyParams{UserID: row.UserID, Source: "local", SourceID: row.ID}); err != nil {
					return err
				}
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
		// A HiringPlug appointment cancelled here (a panel member taken off, say)
		// no longer counts while its Outlook copy waits to be deleted.
		if !e.IsCancelled && e.ShowAs != "free" && start.Before(to) && end.After(from) && (unknown || row.Status == "active") {
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

// byTransaction maps the transaction IDs this connection's creates carried
// (the current and previous generation) to rows not tied to an event yet.
func (s *Service) byTransaction(ctx context.Context, q *store.Queries, c store.CalendarConnection) (map[string]store.CalendarEvent, error) {
	rows, err := q.UnreceiptedCalendarEvents(ctx, store.UnreceiptedCalendarEventsParams{UserID: c.UserID, ConnectionID: c.ID})
	out := make(map[string]store.CalendarEvent, 2*len(rows))
	for _, r := range rows {
		if r.Source == invitationSource && !inviteCalendar(r, c) {
			continue
		}
		out[transactionID(r, r.RemoteGeneration)] = r
		if r.RemoteGeneration > 0 {
			out[transactionID(r, r.RemoteGeneration-1)] = r
		}
	}
	return out, err
}

func (s *Service) adopt(ctx context.Context, tx pgx.Tx, c store.CalendarConnection, row store.CalendarEvent, e graphEvent) (store.CalendarEvent, error) {
	q := store.New(tx)
	if err := q.ReleaseImportedRemote(ctx, store.ReleaseImportedRemoteParams{UserID: c.UserID, ConnectionID: ptr(c.ID), RemoteID: ptr(e.ID)}); err != nil {
		return row, err
	}
	if err := q.AdoptCalendarRemote(ctx, store.AdoptCalendarRemoteParams{ID: row.ID, RemoteID: ptr(e.ID), RemoteKey: e.key(), ConnectionID: ptr(c.ID)}); err != nil {
		return row, err
	}
	if err := s.queuePush(ctx, tx, c.TenantID, row.ID, c.ID); err != nil {
		return row, err
	}
	return q.CalendarEvent(ctx, row.ID)
}

func (s *Service) mergeRemote(ctx context.Context, tx pgx.Tx, row store.CalendarEvent, e graphEvent) error {
	q := store.New(tx)
	if row.Source == invitationSource && row.Teams && row.JoinUrl == nil && !e.IsCancelled {
		if link := e.join(); link != nil {
			if err := q.SaveInterviewJoinLink(ctx, store.SaveInterviewJoinLinkParams{ID: row.ID, JoinUrl: link, ExpectedVersion: row.Version}); err != nil {
				return err
			}
		}
	}
	if text(row.RemoteKey) == text(e.key()) || row.Version != row.SentVersion {
		return nil
	}
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
