package calendars

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) push(ctx context.Context, job jobqueue.Job) error {
	var payload api.CalendarPushJob
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
	return s.network(ctx, job.TenantID, initial.UserID, true, func(ctx context.Context, conn *pgx.Conn) error {
		p := authz.System(job.TenantID)
		var current store.CalendarConnection
		var row store.CalendarEvent
		err := database.InTx(ctx, conn, p, func(t pgx.Tx) error {
			var err error
			current, err = store.New(t).CalendarConnectionByID(ctx, payload.ConnectionId)
			if err != nil {
				return err
			}
			row, err = store.New(t).CalendarEvent(ctx, payload.EventId)
			return err
		})
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.UserID != current.UserID || row.Source == "outlook" || row.SyncStatus == "synced" || row.SyncStatus == "local" || row.SyncStatus == "conflict" || row.SyncStatus == "failed" {
			return nil
		}
		token, err := s.access(ctx, conn, current)
		if err != nil {
			return s.pushFailure(ctx, conn, p, row, current, err)
		}
		path := s.calendarPath(current.CalendarID) + "/events"
		method := "POST"
		var body []byte
		same := row.RemoteConnection != nil && *row.RemoteConnection == current.ID && row.RemoteID != nil
		key := ""
		if same {
			path += "/" + url.PathEscape(*row.RemoteID)
			method = "PATCH"
			if row.RemoteKey != nil {
				key = *row.RemoteKey
			}
		}
		if row.Status == "cancelled" {
			if !same {
				return s.receipt(ctx, conn, p, row, current, graphEvent{})
			}
			method = "DELETE"
		} else {
			body, err = eventBody(row)
			if err != nil {
				return err
			}
		}
		if method == "PATCH" {
			if body, err = patchBody(row, body); err != nil {
				return err
			}
		}
		raw, writeErr := s.request(ctx, method, s.graph(path), token, body, key)
		var provider *providerError
		if errors.As(writeErr, &provider) && provider.Status == 404 && method == "DELETE" {
			writeErr = nil
		}
		if errors.As(writeErr, &provider) && provider.Status == 404 && method == "PATCH" {
			return database.InTx(ctx, conn, p, func(t pgx.Tx) error {
				return store.New(t).MarkCalendarConflict(ctx, store.MarkCalendarConflictParams{ID: row.ID, RemoteSnapshot: []byte(`{"deleted":true}`)})
			})
		}
		if errors.As(writeErr, &provider) && provider.Status == 412 && same {
			latest, readErr := s.oneRemote(ctx, current, *row.RemoteID, token)
			if readErr != nil {
				return s.pushFailure(ctx, conn, p, row, current, readErr)
			}
			snapshot, _ := json.Marshal(latest)
			return database.InTx(ctx, conn, p, func(t pgx.Tx) error {
				return store.New(t).MarkCalendarConflict(ctx, store.MarkCalendarConflictParams{ID: row.ID, RemoteKey: latest.key(), RemoteSnapshot: snapshot})
			})
		}
		if writeErr != nil {
			return s.pushFailure(ctx, conn, p, row, current, writeErr)
		}
		var remote graphEvent
		if method != "DELETE" {
			if len(raw) == 0 && same {
				remote, err = s.oneRemote(ctx, current, *row.RemoteID, token)
			} else {
				err = json.Unmarshal(raw, &remote)
			}
			if err != nil || remote.ID == "" {
				return s.pushFailure(ctx, conn, p, row, current, errors.New("outlook returned no usable event receipt; retry the appointment"))
			}
		}
		return s.receipt(ctx, conn, p, row, current, remote)
	})
}
func (s *Service) receipt(ctx context.Context, conn *pgx.Conn, p authz.Principal, row store.CalendarEvent, c store.CalendarConnection, remote graphEvent) error {
	snapshot, _ := json.Marshal(remote)
	var remoteID *string
	if remote.ID != "" {
		remoteID = &remote.ID
	}
	// The receipt commits independently of the queue transaction. A retry reuses
	// both this ID and the stable Graph transactionId if the worker loses its lease.
	return database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if remoteID != nil { // a sync may have imported this event before its receipt: it is ours
			if err := q.ReleaseImportedRemote(ctx, store.ReleaseImportedRemoteParams{UserID: row.UserID, ConnectionID: ptr(c.ID), RemoteID: remoteID}); err != nil {
				return err
			}
			if err := q.ReleaseImportedBusy(ctx, store.ReleaseImportedBusyParams{UserID: row.UserID, RemoteID: *remoteID}); err != nil {
				return err
			}
		}
		_, err := q.SaveCalendarReceipt(ctx, store.SaveCalendarReceiptParams{ID: row.ID, ConnectionID: ptr(c.ID), SentVersion: row.Version, RemoteID: remoteID, RemoteKey: remote.key(), RemoteSnapshot: snapshot, WebUrl: safeWebURL(remote.WebLink), JoinUrl: remote.join()})
		return err
	})
}
func (s *Service) pushFailure(ctx context.Context, conn *pgx.Conn, p authz.Principal, row store.CalendarEvent, c store.CalendarConnection, cause error) error {
	var provider *providerError
	transient := errors.As(cause, &provider) && (provider.Status == 0 || provider.Status == 429 || provider.Status >= 500)
	if transient {
		delay := provider.Retry
		if delay <= 0 {
			delay = time.Minute
		}
		return jobqueue.Later(s.now().Add(delay))
	}
	if err := database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.CalendarPushFailed(ctx, store.CalendarPushFailedParams{ID: row.ID, ExpectedVersion: row.Version, SyncError: ptr(cause.Error())}); err != nil {
			return err
		}
		if needsReconnect(cause) {
			if _, err := q.FailOutlookRefresh(ctx, store.FailOutlookRefreshParams{ID: c.ID, Sequence: c.SyncSequence, Status: "reconnect", SyncError: ptr(cause.Error())}); err != nil {
				return err
			}
			return q.SaveOutlookAvailability(ctx, store.SaveOutlookAvailabilityParams{UserID: c.UserID, ConnectionID: c.ID, Status: "reconnect"})
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}
