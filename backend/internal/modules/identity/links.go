package identity

// Links emailed to people: invitations, password resets, and reconnect links
// (microsoft_reconnect.go). Only their SHA-256 is kept; each works once, a
// new one ends the old, and using one holds it (two uses at once: one works).

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// newLinkToken returns a random link token and the hash that is stored.
func newLinkToken() (string, string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system's random source failing is not recoverable
	}
	token := hex.EncodeToString(b)
	return token, hashLink(token)
}

func hashLink(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// link makes a new invitation or reset link for a user (ending older ones of
// that kind), in the caller's transaction.
func (m *Module) link(ctx context.Context, q *store.Queries, userID, kind string, valid time.Duration) (string, time.Time, error) {
	if err := q.EndLinks(ctx, store.EndLinksParams{UserID: userID, Kind: kind}); err != nil {
		return "", time.Time{}, err
	}
	token, hash := newLinkToken()
	expires := m.now().Add(valid)
	if _, err := q.InsertLink(ctx, store.InsertLinkParams{UserID: userID, Kind: kind, TokenHash: hash, ExpiresAt: expires}); err != nil {
		return "", time.Time{}, err
	}
	path := map[string]string{"invite": "/#/invite/", "reset": "/#/reset/", "reconnect": "/#/reconnect/"}[kind]
	return m.publicURL + path + token, expires, nil
}

// RequestReset emails a reset link to every active account with the email.
// It answers the same whether or not there is one.
func (m *Module) RequestReset(ctx context.Context, clientAddr, email string) error {
	email = normEmail(email)
	if !m.limits.resetAddr.Allow(clientAddr) || !m.limits.resetEmail.Allow(email) {
		return tooMany()
	}
	accounts, err := store.New(m.pool).AccountsByEmail(ctx, email)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if !a.Active {
			continue
		}
		err := database.InTx(ctx, m.pool, authz.System(a.TenantID), func(tx pgx.Tx) error {
			q := store.New(tx)
			u, err := q.UserAuth(ctx, a.UserID)
			if err != nil {
				return err
			}
			if PasswordRefused(u.MicrosoftRequired, u.Role) { // no link: how to sign in instead
				body := "Hello " + a.UserName + ",\n\nSomeone (hopefully you) asked to reset your HyPlug password at " + a.TenantName +
					".\n\n" + a.TenantName + " signs in with Microsoft, so there is no password to reset: choose Sign in with Microsoft at\n" +
					m.publicURL + "\n\nHyPlug"
				if err := m.mailer.Notify(ctx, tx, a.TenantID, []string{email}, "Signing in to HyPlug", body); err != nil {
					return err
				}
				return audit.Record(ctx, tx, audit.Entry{Action: "user.reset_refused", EntityType: "user", EntityID: a.UserID,
					Changes: map[string]any{"reason": "microsoft_required"}})
			}
			link, _, err := m.link(ctx, q, a.UserID, "reset", ResetValid)
			if err != nil {
				return err
			}
			body := "Hello " + a.UserName + ",\n\nSomeone (hopefully you) asked to reset your HyPlug password at " + a.TenantName +
				".\n\nChoose a new password here (the link works for one hour):\n" + link +
				"\n\nIf it was not you, ignore this email: your password stays as it is.\n\nHyPlug"
			if err := m.mailer.Notify(ctx, tx, a.TenantID, []string{email}, "Reset your HyPlug password", body); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{Action: "user.reset_requested", EntityType: "user", EntityID: a.UserID})
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// LinkInfo is what a link is for, and whether the person can (or must)
// use it with Microsoft instead of a password (MicrosoftLinkState).
type LinkInfo struct {
	Kind, Email, Name, CompanyName, Microsoft string
}

func linkGone() error {
	return apperr.New(api.ErrorCodeLINKNOTVALID, "This link was used, replaced by a newer one, or has expired. Ask for a new one.")
}

// lookupLink finds a valid link's company and user, and loads the user.
// To use the link (hold), the user's row and the link are held, and the link
// checked again: still unused and unexpired (two uses at once: one works).
func (m *Module) lookupLink(ctx context.Context, clientAddr, token string, hold bool, fn func(tx pgx.Tx, q *store.Queries, l store.LinkByHashRow, u store.UserAuthRow) error) error {
	if !m.limits.links.Allow(clientAddr) {
		return tooMany()
	}
	l, err := store.New(m.pool).LinkByHash(ctx, hashLink(token))
	if database.IsNoRows(err) {
		return linkGone()
	}
	if err != nil {
		return err
	}
	return database.InTx(ctx, m.pool, authz.System(l.TenantID), func(tx pgx.Tx) error {
		q := store.New(tx)
		var u store.UserAuthRow
		var err error
		if hold {
			if u, err = holdUser(ctx, q, l.UserID); err != nil {
				return err
			}
			held, err := q.HoldLink(ctx, l.LinkID)
			if database.IsNoRows(err) || (err == nil && (!held.Valid || held.Kind != l.Kind || held.UserID != u.ID)) {
				return linkGone() // used, replaced or expired meanwhile
			}
			if err != nil {
				return err
			}
		} else if u, err = q.UserAuth(ctx, l.UserID); err != nil {
			return err
		}
		if l.Kind != "invite" && !u.Active { // deactivated since
			return linkGone()
		}
		return fn(tx, q, l, u)
	})
}

// Link says what a link is for.
func (m *Module) Link(ctx context.Context, clientAddr, token string) (LinkInfo, error) {
	var out LinkInfo
	err := m.lookupLink(ctx, clientAddr, token, false, func(tx pgx.Tx, q *store.Queries, l store.LinkByHashRow, u store.UserAuthRow) error {
		t, err := q.GetTenant(ctx, l.TenantID)
		if err != nil {
			return err
		}
		s, err := q.MicrosoftSettings(ctx)
		out = LinkInfo{Kind: l.Kind, Email: u.Email, Name: u.Name, CompanyName: t.Name,
			Microsoft: MicrosoftLinkState(s.MicrosoftSignIn && m.ms != nil, s.MicrosoftRequired, u.Role)}
		if l.Kind == "reconnect" && out.Microsoft != "none" { // Microsoft only, owners too
			out.Microsoft = "required"
		}
		return err
	})
	return out, err
}

// UseLink sets the password chosen with a link (an invitation also activates
// the account, and may correct the name), ends the link and the user's other
// sessions, and signs them in.
func (m *Module) UseLink(ctx context.Context, clientAddr, token, password string, name *string) (Session, error) {
	var out Session
	var next *apperr.Error // the password is saved; two-step sign-in still asks for its code
	err := m.lookupLink(ctx, clientAddr, token, true, func(tx pgx.Tx, q *store.Queries, l store.LinkByHashRow, u store.UserAuthRow) error {
		if l.Kind == "reconnect" { // only to sign in with Microsoft
			return linkGone()
		}
		if l.Kind != "invite" {
			name = nil
		} else if name != nil {
			n := strings.Join(strings.Fields(*name), " ")
			name = &n
			if n == "" {
				name = nil
			}
		}
		if PasswordRefused(u.MicrosoftRequired, u.Role) { // they accept an invitation with Microsoft
			return microsoftRequired()
		}
		if problem := PasswordProblem(password, u.Email, u.Name); problem != "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "password", Message: problem}})
		}
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		now, err := sessionsEndedAt(ctx, q, u)
		if err != nil {
			return err
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{ID: u.ID, PasswordHash: &hash, ValidAfter: &now, Name: name}); err != nil {
			return err
		}
		if err := q.EndLinks(ctx, store.EndLinksParams{UserID: u.ID, Kind: l.Kind}); err != nil {
			return err
		}
		action := "user.password_reset"
		if l.Kind == "invite" {
			action = "user.joined"
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "user", EntityID: u.ID}); err != nil {
			return err
		}
		u.TokensValidAfter = &now // a two-step challenge is for this password only
		if next, err = m.nextStep(ctx, q, u); err != nil || next != nil {
			return err
		}
		out, err = m.session(l.TenantID, u.ID, authn.MethodPassword, now)
		return err
	})
	if err == nil && next != nil {
		return Session{}, next
	}
	return out, err
}
