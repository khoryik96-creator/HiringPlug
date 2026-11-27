package calendars

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

func (s *Service) appIn(ctx context.Context, p authz.Principal) (Application, error) {
	var app Application
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		app, err = s.deps.Apps.MicrosoftApp(ctx, tx, p.TenantID)
		return err
	})
	return app, err
}
func authorizationProblem(message string) error {
	return apperr.Validation(apperr.FieldErrors{{Field: "outlook", Message: message}})
}
func (s *Service) authorize(ctx context.Context, p authz.Principal) (string, error) {
	if len(s.config.KeysSecret) < 32 {
		return "", authorizationProblem("The server's secure connection storage is not configured.")
	}
	app, err := s.appIn(ctx, p)
	if err != nil {
		return "", err
	}
	if app.ClientID == "" || app.ClientSecret == "" {
		return "", authorizationProblem("Ask an owner or admin to configure the Microsoft application in Settings > Email connection.")
	}
	expires := s.now().Add(10 * time.Minute)
	state := s.signState(stateClaim{Tenant: p.TenantID, User: p.UserID, Nonce: randomToken(), Expires: expires.Unix()})
	verifier := randomToken()
	challenge := sha256.Sum256([]byte(verifier))
	sealed, err := s.seal(p.TenantID, p.UserID, "verifier", verifier)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockCalendarUser(ctx, p.UserID); err != nil {
			return err
		}
		return q.SaveCalendarState(ctx, store.SaveCalendarStateParams{StateHash: digest(state), AppClientID: app.ClientID, SealedVerifier: sealed, ExpiresAt: expires})
	})
	if err != nil {
		return "", err
	}
	target, _ := url.Parse(s.config.Provider.AuthorizeURL)
	q := target.Query()
	q.Set("client_id", app.ClientID)
	q.Set("redirect_uri", s.callbackURL())
	q.Set("scope", calendarScope)
	q.Set("response_type", "code")
	q.Set("state", state)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("prompt", "select_account")
	target.RawQuery = q.Encode()
	return target.String(), nil
}
func (s *Service) park(ctx context.Context, state, code, denied string) (string, error) {
	claim, err := s.readState(state)
	if err != nil {
		return "", err
	}
	p := authz.NewPrincipalWithAccess(claim.Tenant, claim.User, "", "", api.RoleSourcer, []api.Permission{api.PermissionCalendarsUse}, authz.Scope{Users: []string{}})
	var sealedCode, ticketHash *string
	ticket := ""
	if code != "" && denied == "" {
		sealed, err := s.seal(claim.Tenant, claim.User, "code", code)
		if err != nil {
			return "", err
		}
		sealedCode = &sealed
		ticket = randomToken()
		ticketHash = ptr(digest(ticket))
	}
	var changed int64
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		changed, err = store.New(tx).ParkCalendarCode(ctx, store.ParkCalendarCodeParams{StateHash: digest(state), SealedCode: sealedCode, TicketHash: ticketHash})
		return err
	})
	if err != nil || changed != 1 || ticket == "" {
		return "", errors.New("calendar authorization was not completed or expired")
	}
	return ticket, nil
}
func (s *Service) complete(ctx context.Context, p authz.Principal, ticket string) error {
	problem := authorizationProblem("Calendar sign-in was not completed or expired. Connect again from this account.")
	var saved store.CalendarOauthState
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		saved, err = store.New(tx).ParkedCalendarCode(ctx, ptr(digest(ticket)))
		return err
	})
	if database.IsNoRows(err) || saved.SealedCode == nil {
		return problem
	}
	if err != nil {
		return err
	}
	var code, verifier string
	if err = s.open(p.TenantID, p.UserID, "code", *saved.SealedCode, &code); err != nil {
		return problem
	}
	if err = s.open(p.TenantID, p.UserID, "verifier", saved.SealedVerifier, &verifier); err != nil {
		return problem
	}
	app, err := s.appIn(ctx, p)
	if err != nil {
		return err
	}
	if app.ClientID != saved.AppClientID {
		return authorizationProblem("The Microsoft application changed during sign-in. Connect again.")
	}
	secret, err := s.exchange(ctx, app, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.callbackURL()}, "code_verifier": {verifier}})
	if err != nil {
		return authorizationProblem(err.Error())
	}
	if !writeScope(secret.Scope) || secret.RefreshToken == "" {
		return authorizationProblem("Approve Calendars.ReadWrite and offline access for this application, then reconnect Outlook.")
	}
	profile, err := s.profile(ctx, secret.AccessToken)
	if err != nil {
		return authorizationProblem(err.Error())
	}
	sealed, err := s.seal(p.TenantID, p.UserID, "tokens", secret)
	if err != nil {
		return err
	}
	return s.network(ctx, p.TenantID, p.UserID, false, func(ctx context.Context, conn *pgx.Conn) error {
		// Another account: the interviews put into the old one come out of it
		// (they would never be updated again there), once the new one is
		// saved: a connect that fails leaves the old calendar as it was.
		var old store.CalendarConnection
		var oldHolds []store.CalendarEvent
		if o, err := s.liveConnection(ctx, conn, p); err == nil && (o.SubjectID != profile.ID || o.CalendarID != profile.CalendarID) {
			if oldHolds, err = s.pushedHolds(ctx, conn, o); err != nil {
				return err
			}
			old = o
		}
		err := database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
			q := store.New(tx)
			if err := q.LockCalendarUser(ctx, p.UserID); err != nil {
				return err
			}
			currentApp, err := s.deps.Apps.MicrosoftApp(ctx, tx, p.TenantID)
			if err != nil {
				return err
			}
			if currentApp.ClientID != app.ClientID {
				return problem
			}
			claimed, err := q.ClaimCalendarCode(ctx, saved.StateHash)
			if err != nil {
				return err
			}
			if claimed != 1 {
				return problem
			}
			previous, oldErr := q.PreviousCalendarConnection(ctx, p.UserID)
			if oldErr != nil && !database.IsNoRows(oldErr) {
				return oldErr
			}
			conn, err := q.SaveCalendarConnection(ctx, store.SaveCalendarConnectionParams{SubjectID: profile.ID, Address: profile.Address, AppClientID: app.ClientID, CalendarID: profile.CalendarID, CalendarName: profile.CalendarName, TeamsAvailable: profile.Teams, SealedTokens: sealed})
			if database.IsUniqueViolation(err, "calendar_account_owner") {
				return authorizationProblem("This Outlook calendar is already connected to another HyPlug account in this agency.")
			}
			if err != nil {
				return err
			}
			if err := q.ClearImportedEvents(ctx); err != nil {
				return err
			}
			same := oldErr == nil && previous.SubjectID == conn.SubjectID && previous.CalendarID == conn.CalendarID
			if same {
				err = q.RebindCalendarTargets(ctx, store.RebindCalendarTargetsParams{ConnectionID: ptr(conn.ID), PreviousConnection: ptr(previous.ID)})
			} else {
				err = q.ResetCalendarTargets(ctx)
				if err == nil {
					err = q.BlockReroutedCalendarInvites(ctx)
				}
			}
			if err != nil {
				return err
			}
			if err := q.SaveOutlookAvailability(ctx, store.SaveOutlookAvailabilityParams{UserID: p.UserID, ConnectionID: conn.ID, Status: "pending"}); err != nil {
				return err
			}
			if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindCalendarRefresh, api.CalendarRefreshJob{ConnectionId: conn.ID, Sequence: int(conn.SyncSequence)}); err != nil {
				return err
			}
			if s.deps.Sources != nil {
				meetings, err := s.deps.Sources.MeetingsFor(ctx, tx, p.UserID, s.now().Add(-24*time.Hour), s.now().Add(94*24*time.Hour))
				if err != nil {
					return err
				}
				for _, m := range meetings {
					if _, err := s.holdIn(ctx, tx, p.TenantID, p.UserID, m); err != nil {
						return err
					}
				}
			}
			if err := q.PrepareCalendarTargets(ctx); err != nil {
				return err
			}
			pending, err := q.PendingCalendarEvents(ctx, p.UserID)
			if err != nil {
				return err
			}
			for _, id := range pending {
				if err := s.queuePush(ctx, tx, p.TenantID, id, conn.ID); err != nil {
					return err
				}
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "calendar.connected", EntityType: "user", EntityID: p.UserID, Changes: map[string]any{"provider": "microsoft"}})
		})
		if err == nil && len(oldHolds) > 0 {
			if token, tokenErr := s.tokenOf(ctx, old); tokenErr == nil {
				s.withdraw(ctx, token, old, oldHolds)
			}
		}
		return err
	})
}
func (s *Service) disconnect(ctx context.Context, p authz.Principal) error {
	return s.network(ctx, p.TenantID, p.UserID, false, func(ctx context.Context, conn *pgx.Conn) error {
		// The interviews and bookings HyPlug put into Outlook come out:
		// nothing would update them once disconnected. (Events the person
		// made themselves stay: they are theirs.) Only those Outlook really
		// deleted are forgotten; the others keep their link, so reconnecting
		// the account updates them instead of making copies.
		var deleted []string
		if old, err := s.liveConnection(ctx, conn, p); err == nil {
			if holds, err := s.pushedHolds(ctx, conn, old); err == nil && len(holds) > 0 {
				if token, err := s.access(ctx, conn, old); err == nil {
					deleted = s.withdraw(ctx, token, old, holds)
				}
			}
		}
		return database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
			q := store.New(tx)
			if err := q.LockCalendarUser(ctx, p.UserID); err != nil {
				return err
			}
			if len(deleted) > 0 {
				if err := q.ForgetPushedHolds(ctx, deleted); err != nil {
					return err
				}
			}
			if err := q.DeleteCalendarConnection(ctx); err != nil {
				return err
			}
			if err := q.ClearCalendarState(ctx); err != nil {
				return err
			}
			if err := q.ClearOutlookAvailability(ctx); err != nil {
				return err
			}
			if err := q.ClearOutlookBusy(ctx, p.UserID); err != nil {
				return err
			}
			if err := q.ClearImportedEvents(ctx); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "calendar.disconnected", EntityType: "user", EntityID: p.UserID})
		})
	})
}
func (s *Service) askRefresh(ctx context.Context, p authz.Principal) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockCalendarUser(ctx, p.UserID); err != nil {
			return err
		}
		row, err := q.AskOutlookRefresh(ctx)
		if database.IsNoRows(err) {
			return apperr.New(api.ErrorCodeCALENDARNOTCONNECTED, "Connect Outlook first.")
		}
		if err != nil {
			return err
		}
		_, err = jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindCalendarRefresh, api.CalendarRefreshJob{ConnectionId: row.ID, Sequence: int(row.SyncSequence)})
		return err
	})
}

// liveConnection is the user's current Outlook connection, if any.
func (s *Service) liveConnection(ctx context.Context, conn *pgx.Conn, p authz.Principal) (store.CalendarConnection, error) {
	var row store.CalendarConnection
	err := database.InTx(ctx, conn, p, func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).CalendarConnection(ctx, p.UserID)
		return err
	})
	return row, err
}

// pushedHolds are the interview and booking holds HyPlug put into c.
func (s *Service) pushedHolds(ctx context.Context, conn *pgx.Conn, c store.CalendarConnection) ([]store.CalendarEvent, error) {
	var holds []store.CalendarEvent
	err := database.InTx(ctx, conn, authz.System(c.TenantID), func(tx pgx.Tx) error {
		var err error
		holds, err = store.New(tx).PushedSourceHolds(ctx, store.PushedSourceHoldsParams{UserID: c.UserID, ConnectionID: ptr(c.ID)})
		return err
	})
	return holds, err
}

// withdraw deletes holds from an Outlook connection, as far as Outlook lets
// it, and says which are gone (deleted now, or already).
func (s *Service) withdraw(ctx context.Context, token string, c store.CalendarConnection, holds []store.CalendarEvent) []string {
	var deleted []string
	for _, h := range holds {
		_, err := s.request(ctx, "DELETE", s.graph(s.calendarPath(c.CalendarID)+"/events/"+url.PathEscape(*h.RemoteID)), token, nil, "")
		var provider *providerError
		if err == nil || (errors.As(err, &provider) && provider.Status == 404) {
			deleted = append(deleted, h.ID)
		}
	}
	return deleted
}

// tokenOf is an access token for a connection that may no longer be the
// current one (an account just replaced): its sealed tokens, refreshed if
// need be, never saved back.
func (s *Service) tokenOf(ctx context.Context, c store.CalendarConnection) (string, error) {
	var secret tokens
	if err := s.open(c.TenantID, c.UserID, "tokens", c.SealedTokens, &secret); err != nil {
		return "", err
	}
	if secret.ExpiresAt.Sub(s.now()) > time.Minute {
		return secret.AccessToken, nil
	}
	if secret.RefreshToken == "" {
		return "", errors.New("the old Outlook authorization has expired")
	}
	app, err := s.appIn(ctx, authz.System(c.TenantID))
	if err != nil || app.ClientID != c.AppClientID {
		return "", errors.New("the Microsoft application changed")
	}
	next, err := s.exchange(ctx, app, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {secret.RefreshToken}})
	return next.AccessToken, err
}
