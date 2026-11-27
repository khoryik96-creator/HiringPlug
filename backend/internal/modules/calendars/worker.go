package calendars

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

type reconnectError struct{ message string }

func (e *reconnectError) Error() string { return e.message }
func needsReconnect(err error) bool {
	var r *reconnectError
	var p *providerError
	return errors.As(err, &r) || (errors.As(err, &p) && (p.Status == 401 || p.Status == 403))
}

// networkConns bounds the database connections Outlook work holds at once
// (each holds one while it waits on Microsoft); the rest wait their turn.
const networkConns = 4

// A pooled connection serializes this user's provider operations without
// holding business-row locks or a transaction while Microsoft answers; at
// most networkConns background ones are held at once, so Outlook syncs never
// starve the pool (they used to open a new database connection per job).
// Background work (sync, push) waits for a slot; a person connecting or
// disconnecting does not queue behind it.
func (s *Service) network(ctx context.Context, tenant, user string, background bool, fn func(context.Context, *pgx.Conn) error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if background {
		select {
		case s.netSlots <- struct{}{}:
			defer func() { <-s.netSlots }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	pc, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	conn := pc.Conn()
	q := store.New(conn)
	if err := q.LockCalendarNetwork(ctx, store.LockCalendarNetworkParams{TenantID: tenant, UserID: user}); err != nil {
		pc.Release()
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if q.UnlockCalendarNetwork(cleanup, store.UnlockCalendarNetworkParams{TenantID: tenant, UserID: user}) != nil {
			_ = pc.Hijack().Close(cleanup) // never return a connection that may still hold the lock
			return
		}
		pc.Release()
	}()
	return fn(ctx, conn)
}

// connectionByID reads a connection in its own short transaction (jobs that
// talk to Outlook hold none while they wait).
func (s *Service) connectionByID(ctx context.Context, tenant, id string) (store.CalendarConnection, error) {
	var row store.CalendarConnection
	err := database.InTx(ctx, s.pool, authz.System(tenant), func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).CalendarConnectionByID(ctx, id)
		return err
	})
	return row, err
}

func (s *Service) access(ctx context.Context, conn *pgx.Conn, expected store.CalendarConnection) (string, error) {
	var token string
	err := database.InTx(ctx, conn, authz.System(expected.TenantID), func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockCalendarUser(ctx, expected.UserID); err != nil {
			return err
		}
		row, err := q.CalendarConnection(ctx, expected.UserID)
		if err != nil {
			return err
		}
		if row.ID != expected.ID {
			return &reconnectError{"The Outlook connection changed; stale operations were cancelled"}
		}
		active, err := s.deps.Users.CalendarUserActive(ctx, tx, row.UserID)
		if err != nil {
			return err
		}
		if !active {
			return &reconnectError{"Calendar access is no longer enabled for this HyPlug user"}
		}
		app, err := s.deps.Apps.MicrosoftApp(ctx, tx, row.TenantID)
		if err != nil {
			return err
		}
		if app.ClientID != row.AppClientID || app.ClientSecret == "" {
			return &reconnectError{"The Microsoft application changed; reconnect Outlook"}
		}
		var secret tokens
		if err := s.open(row.TenantID, row.UserID, "tokens", row.SealedTokens, &secret); err != nil {
			return &reconnectError{"Outlook credentials cannot be opened; reconnect Outlook"}
		}
		if secret.ExpiresAt.Sub(s.now()) > time.Minute {
			token = secret.AccessToken
			return nil
		}
		if secret.RefreshToken == "" {
			return &reconnectError{"Outlook authorization expired; reconnect Outlook"}
		}
		next, err := s.exchange(ctx, app, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {secret.RefreshToken}})
		if err != nil {
			var provider *providerError
			if errors.As(err, &provider) && provider.Status == 400 {
				return &reconnectError{"Outlook authorization expired; reconnect Outlook"}
			}
			return err
		}
		if next.RefreshToken == "" {
			next.RefreshToken = secret.RefreshToken
		}
		if next.Scope == "" {
			next.Scope = secret.Scope
		}
		if !writeScope(next.Scope) {
			return &reconnectError{"Approve calendar permissions and reconnect Outlook"}
		}
		sealed, err := s.seal(row.TenantID, row.UserID, "tokens", next)
		if err != nil {
			return err
		}
		if _, err := q.RefreshCalendarTokens(ctx, store.RefreshCalendarTokensParams{ID: row.ID, SealedTokens: sealed}); err != nil {
			return err
		}
		token = next.AccessToken
		return nil
	})
	return token, err
}
