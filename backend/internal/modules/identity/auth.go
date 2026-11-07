package identity

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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/ratelimit"
)

// SessionLength is how long a sign-in lasts.
const SessionLength = 12 * time.Hour

// How long links work.
const (
	InviteValid = 7 * 24 * time.Hour
	ResetValid  = time.Hour
)

// limits slow down guessing: per client address, and per email or user.
type limits struct {
	signInAddr, signInEmail, resetAddr, resetEmail, links, change *ratelimit.Limiter
}

func newLimits(now func() time.Time) limits {
	l := func(max int, window time.Duration, dayMax int) *ratelimit.Limiter {
		return ratelimit.New(now, ratelimit.Limit{Window: window, Max: max}, ratelimit.Limit{Window: 24 * time.Hour, Max: dayMax})
	}
	return limits{
		signInAddr: l(30, 15*time.Minute, 300), signInEmail: l(10, 15*time.Minute, 50),
		resetAddr: l(10, 15*time.Minute, 50), resetEmail: l(3, time.Hour, 10),
		links: l(30, 15*time.Minute, 200), change: l(10, 15*time.Minute, 50),
	}
}

func tooMany() error {
	return apperr.New(api.ErrorCodeTOOMANYATTEMPTS, "Too many attempts. Wait a few minutes and try again.")
}

// Session is a sign-in token and when it ends.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// sessionsEndedAt is when a password change ends the user's sessions: the
// next whole second, so every token issued until now (tokens carry whole
// seconds) is before it, and the new session, issued at it, is not.
func sessionsEndedAt() time.Time { return time.Now().Truncate(time.Second).Add(time.Second) }

// session mints a token issued at now.
func (m *Module) session(tenantID, userID string, now time.Time) (Session, error) {
	token, err := m.verifier.MintAt(tenantID, userID, now, SessionLength)
	return Session{Token: token, ExpiresAt: now.Add(SessionLength)}, err
}

func normEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// SignIn checks an email and password. When they match accounts at several
// companies, company (the careers page slug) chooses one.
func (m *Module) SignIn(ctx context.Context, clientAddr, email, password, company string) (Session, error) {
	email = normEmail(email)
	if !m.limits.signInAddr.Allow(clientAddr) || !m.limits.signInEmail.Allow(email) {
		return Session{}, tooMany()
	}
	accounts, err := store.New(m.pool).AccountsByEmail(ctx, email)
	if err != nil {
		return Session{}, err
	}
	var matches []store.AccountsByEmailRow
	checked := false
	for _, a := range accounts {
		if !a.Active || a.PasswordHash == "" || (company != "" && a.CareersSlug != company) {
			continue
		}
		checked = true
		if CheckPassword(a.PasswordHash, password) {
			matches = append(matches, a)
		}
	}
	if !checked {
		CheckPassword(dummyHash(), password) // as slow as a real check
	}
	switch len(matches) {
	case 0:
		return Session{}, apperr.New(api.ErrorCodeSIGNINFAILED, "The email or password is wrong.")
	case 1:
	default:
		companies := make([]map[string]string, len(matches))
		for i, a := range matches {
			companies[i] = map[string]string{"slug": a.CareersSlug, "name": a.TenantName}
		}
		return Session{}, apperr.New(api.ErrorCodeSIGNINCHOOSECOMPANY, "Your account is at several companies: choose one.").WithDetail("companies", companies)
	}
	a := matches[0]
	now := time.Now().Truncate(time.Second)
	err = database.InTx(ctx, m.pool, authz.NewPrincipal(a.TenantID, a.UserID, email, a.UserName, ""), func(tx pgx.Tx) error {
		if err := store.New(tx).SignedIn(ctx, a.UserID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.signed_in", EntityType: "user", EntityID: a.UserID})
	})
	if err != nil {
		return Session{}, err
	}
	return m.session(a.TenantID, a.UserID, now)
}

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
	expires := time.Now().Add(valid)
	if _, err := q.InsertLink(ctx, store.InsertLinkParams{UserID: userID, Kind: kind, TokenHash: hash, ExpiresAt: expires}); err != nil {
		return "", time.Time{}, err
	}
	path := "/#/reset/"
	if kind == "invite" {
		path = "/#/invite/"
	}
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
			link, _, err := m.link(ctx, store.New(tx), a.UserID, "reset", ResetValid)
			if err != nil {
				return err
			}
			body := "Hello " + a.UserName + ",\n\nSomeone (hopefully you) asked to reset your HiringPlug password at " + a.TenantName +
				".\n\nChoose a new password here (the link works for one hour):\n" + link +
				"\n\nIf it was not you, ignore this email: your password stays as it is.\n\nHiringPlug"
			if err := m.mailer.Notify(ctx, tx, a.TenantID, []string{email}, "Reset your HiringPlug password", body); err != nil {
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

// LinkInfo is what a link is for.
type LinkInfo struct {
	Kind, Email, Name, CompanyName string
}

func linkGone() error {
	return apperr.New(api.ErrorCodeLINKNOTVALID, "This link was used, replaced by a newer one, or has expired. Ask for a new one.")
}

// lookupLink finds a valid link's company and user, and loads the user.
func (m *Module) lookupLink(ctx context.Context, clientAddr, token string, fn func(tx pgx.Tx, q *store.Queries, l store.LinkByHashRow, u store.UserAuthRow) error) error {
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
		u, err := q.UserAuth(ctx, l.UserID)
		if err != nil {
			return err
		}
		if l.Kind == "reset" && !u.Active { // deactivated since
			return linkGone()
		}
		return fn(tx, q, l, u)
	})
}

// Link says what a link is for.
func (m *Module) Link(ctx context.Context, clientAddr, token string) (LinkInfo, error) {
	var out LinkInfo
	err := m.lookupLink(ctx, clientAddr, token, func(tx pgx.Tx, q *store.Queries, l store.LinkByHashRow, u store.UserAuthRow) error {
		t, err := q.GetTenant(ctx, l.TenantID)
		out = LinkInfo{Kind: l.Kind, Email: u.Email, Name: u.Name, CompanyName: t.Name}
		return err
	})
	return out, err
}

// UseLink sets the password chosen with a link (an invitation also activates
// the account, and may correct the name), ends the link and the user's other
// sessions, and signs them in.
func (m *Module) UseLink(ctx context.Context, clientAddr, token, password string, name *string) (Session, error) {
	var out Session
	err := m.lookupLink(ctx, clientAddr, token, func(tx pgx.Tx, q *store.Queries, l store.LinkByHashRow, u store.UserAuthRow) error {
		if l.Kind != "invite" {
			name = nil
		} else if name != nil {
			n := strings.Join(strings.Fields(*name), " ")
			name = &n
			if n == "" {
				name = nil
			}
		}
		if problem := PasswordProblem(password, u.Email, u.Name); problem != "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "password", Message: problem}})
		}
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		now := sessionsEndedAt()
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
		out, err = m.session(l.TenantID, u.ID, now)
		return err
	})
	return out, err
}

// ChangePassword changes the signed-in user's password, signs out their
// other sessions, and returns a new session for this one.
func (m *Module) ChangePassword(ctx context.Context, p authz.Principal, current, next string) (Session, error) {
	if !m.limits.change.Allow(p.UserID) {
		return Session{}, tooMany()
	}
	var out Session
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, p.UserID)
		if err != nil {
			return err
		}
		if u.PasswordHash == nil || !CheckPassword(*u.PasswordHash, current) {
			return apperr.Validation(apperr.FieldErrors{{Field: "currentPassword", Message: "is not your current password"}})
		}
		if problem := PasswordProblem(next, u.Email, u.Name); problem != "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "newPassword", Message: problem}})
		}
		hash, err := HashPassword(next)
		if err != nil {
			return err
		}
		now := sessionsEndedAt()
		if err := q.SetPassword(ctx, store.SetPasswordParams{ID: u.ID, PasswordHash: &hash, ValidAfter: &now}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "user.password_changed", EntityType: "user", EntityID: u.ID}); err != nil {
			return err
		}
		out, err = m.session(p.TenantID, p.UserID, now)
		return err
	})
	return out, err
}

// PasswordLink makes a link for a user to choose a password, valid for
// valid: for the command line (the first owner, or anyone without email).
func (m *Module) PasswordLink(ctx context.Context, tenantID, userID string, valid time.Duration) (string, error) {
	var link string
	err := database.InTx(ctx, m.pool, authz.System(tenantID), func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, userID)
		if err != nil {
			return err
		}
		kind := "reset"
		if u.Invited || u.PasswordHash == nil {
			kind = "invite"
		}
		link, _, err = m.link(ctx, q, userID, kind, valid)
		return err
	})
	return link, err
}

// SignOut ends the request's session: its token is refused from now on.
// Tokens without an ID (from before, or made by hp token) simply expire.
func (m *Module) SignOut(ctx context.Context, p authz.Principal) error {
	sess := p.Session()
	if sess.ID == "" {
		return nil
	}
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.PruneRevokedSessions(ctx); err != nil { // those past their expiry need no keeping
			return err
		}
		if err := q.RevokeSession(ctx, store.RevokeSessionParams{ID: sess.ID, UserID: p.UserID, ExpiresAt: sess.Expires}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.signed_out", EntityType: "user", EntityID: p.UserID})
	})
}
