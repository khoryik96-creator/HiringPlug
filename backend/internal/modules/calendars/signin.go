package calendars

// Connecting the Outlook calendar when someone signs in with Microsoft (the
// company connects Outlook at sign-in): the sign-in's Graph tokens, of the
// company's Microsoft app, connect it as Connect Outlook would. A calendar of
// the same account gets the new tokens (and syncs again if it was waiting to
// be reconnected); one of another account is theirs to change: left as it is.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// SignInGrant is the Outlook part of a Microsoft sign-in.
type SignInGrant struct {
	ClientID                  string
	AccessToken, RefreshToken string
	ExpiresIn                 int    // seconds
	Scope                     string // what Microsoft granted
}

// What connecting at sign-in did.
const (
	SignInConnected = "connected" // a calendar connected where there was none
	SignInRefreshed = "refreshed" // the same calendar, its tokens renewed
	SignInLeft      = "left"      // another account's calendar stays connected
	SignInSkipped   = "skipped"   // another app, or the calendar not granted
)

// ConnectFromSignIn connects (or refreshes) the person's Outlook calendar
// with the Outlook part of their Microsoft sign-in, and says what it did.
func (m *Module) ConnectFromSignIn(ctx context.Context, p authz.Principal, g SignInGrant) (string, error) {
	s := m.service
	if s.deps.Apps == nil {
		return SignInSkipped, nil
	}
	app, err := s.appIn(ctx, p)
	if err != nil {
		return "", err
	}
	if app.ClientID == "" || app.ClientID != g.ClientID || g.RefreshToken == "" || !writeScope(g.Scope) {
		return SignInSkipped, nil
	}
	if g.ExpiresIn < 1 || g.ExpiresIn > 86400 {
		return "", errors.New("calendars: the sign-in's calendar authorization is not valid")
	}
	secret := tokens{AccessToken: g.AccessToken, RefreshToken: g.RefreshToken, Scope: g.Scope, ExpiresAt: s.now().Add(time.Duration(g.ExpiresIn) * time.Second)}
	profile, err := s.profile(ctx, secret.AccessToken)
	if err != nil {
		return "", err
	}
	var current store.CalendarConnection
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		current, err = store.New(tx).CalendarConnection(ctx, p.UserID)
		return err
	})
	switch {
	case database.IsNoRows(err) || (err == nil && current.Status == "disconnected"):
		problem := authorizationProblem("The Microsoft application changed during sign-in.")
		if err := s.connect(ctx, p, app, secret, profile, problem, nil); err != nil {
			return "", err
		}
		return SignInConnected, nil
	case err != nil:
		return "", err
	case current.SubjectID != profile.ID || current.CalendarID != profile.CalendarID || current.AppClientID != app.ClientID:
		return SignInLeft, nil
	}
	sealed, err := s.seal(p.TenantID, p.UserID, "tokens", secret)
	if err != nil {
		return "", err
	}
	return SignInRefreshed, database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockCalendarUser(ctx, p.UserID); err != nil {
			return err
		}
		if _, err := q.RefreshCalendarTokens(ctx, store.RefreshCalendarTokensParams{ID: current.ID, SealedTokens: sealed}); err != nil {
			return err
		}
		if strings.EqualFold(current.Status, "reconnect") { // it was waiting for this: sync again
			row, err := q.AskOutlookRefresh(ctx)
			if err != nil {
				return err
			}
			if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindCalendarRefresh, api.CalendarRefreshJob{ConnectionId: row.ID, Sequence: int(row.SyncSequence)}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "calendar.reconnected", EntityType: "user", EntityID: p.UserID,
			Changes: map[string]any{"provider": "microsoft", "via": "microsoft_sign_in"}})
	})
}
