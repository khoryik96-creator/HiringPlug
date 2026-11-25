package identity

import (
	"context"
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
	signInAddr, signInPair, signInEmail, resetAddr, resetEmail, links, change *ratelimit.Limiter
	twoStep, twoStepUser                                                      *ratelimit.Limiter // codes, per address and per person
	// Signing in with Microsoft, per address: its own limits, not the
	// password's (no password is guessed; Microsoft checks the person).
	msStart, msCallback, msComplete *ratelimit.Limiter
}

// signIn reports whether another sign-in attempt for email from addr is
// allowed. Guesses are counted per address and per address-and-email, so
// someone guessing a colleague's password locks out only themselves, not
// the colleague signing in from elsewhere. A much higher limit per email
// (from every address together) still bounds guessing from many addresses.
func (l limits) signIn(addr, email string) bool {
	return l.signInAddr.Allow(addr) && l.signInPair.Allow(addr+"|"+email) && l.signInEmail.Allow(email)
}

func newLimits(now func() time.Time) limits {
	l := func(max int, window time.Duration, dayMax int) *ratelimit.Limiter {
		return ratelimit.New(now, ratelimit.Limit{Window: window, Max: max}, ratelimit.Limit{Window: 24 * time.Hour, Max: dayMax})
	}
	return limits{
		signInAddr: l(30, 15*time.Minute, 300), signInPair: l(10, 15*time.Minute, 50), signInEmail: l(200, 15*time.Minute, 2000),
		resetAddr: l(10, 15*time.Minute, 50), resetEmail: l(3, time.Hour, 10),
		links: l(30, 15*time.Minute, 200), change: l(10, 15*time.Minute, 50),
		twoStep: l(30, 15*time.Minute, 300), twoStepUser: l(10, 15*time.Minute, 50),
		msStart: l(120, 15*time.Minute, 1000), msCallback: l(300, 15*time.Minute, 3000), msComplete: l(300, 15*time.Minute, 3000),
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

// databaseSecond is the database's clock, to the second (tokens carry whole
// seconds), as of the transaction's start: when a session is issued.
// Sessions are issued, and ended (tokens_valid_after,
// microsoft_sessions_after, some of them set in SQL), by the database's
// clock alone: by two clocks a little apart, a session issued just before an
// end could outlive it. An end is cut off by its clock now (CutOffSecond),
// not at its transaction's start: a sign-in may have committed while it
// waited for the person's row. A session's issue time is never before the
// last end (the row, held, read after it).
func databaseSecond(ctx context.Context, q *store.Queries) (time.Time, error) {
	return q.DatabaseSecond(ctx)
}

// sessionsEndedAt is when a password change ends u's sessions: the next
// whole second by the database's clock now, so every token issued until now
// is before it, and the new session, issued at it, is not; and after every
// earlier end (a session may have been issued at one, in this second). u is
// held (holdUser).
func sessionsEndedAt(ctx context.Context, q *store.Queries, u store.UserAuthRow) (time.Time, error) {
	now, err := q.CutOffSecond(ctx)
	end := now.Add(time.Second)
	for _, after := range []*time.Time{u.TokensValidAfter, u.MicrosoftSessionsAfter} {
		if after != nil && !end.After(*after) {
			end = after.Add(time.Second)
		}
	}
	return end, err
}

// session mints a token issued at now, saying how the person signed in
// (authn.MethodPassword or authn.MethodMicrosoft).
func (m *Module) session(tenantID, userID, method string, now time.Time) (Session, error) {
	token, err := m.verifier.MintSession(tenantID, userID, method, now, SessionLength)
	return Session{Token: token, ExpiresAt: now.Add(SessionLength)}, err
}

func microsoftRequired() error {
	return apperr.New(api.ErrorCodeMICROSOFTREQUIRED, "Your company signs in with Microsoft: use Sign in with Microsoft.")
}

func normEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// holdUser holds the person's row until the transaction ends (a password
// change, a deactivation or an end of their sessions waits, or was made
// before) and reads it: what was checked before the transaction is checked
// again on what it reads.
func holdUser(ctx context.Context, q *store.Queries, userID string) (store.UserAuthRow, error) {
	if _, err := q.HoldUser(ctx, userID); err != nil {
		return store.UserAuthRow{}, err
	}
	return q.UserAuth(ctx, userID)
}

// SignIn checks an email and password. When they match accounts at several
// companies, company (the careers page slug) chooses one.
func (m *Module) SignIn(ctx context.Context, clientAddr, email, password, company string) (Session, error) {
	email = normEmail(email)
	if !m.limits.signIn(clientAddr, email) {
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
	matches, refused, err := m.passwordAllowed(ctx, matches)
	if err != nil {
		return Session{}, err
	}
	switch len(matches) {
	case 0:
		if refused { // the password was right: say what to do instead
			return Session{}, microsoftRequired()
		}
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
	var now time.Time
	var next *apperr.Error // a code is needed first
	err = database.InTx(ctx, m.pool, authz.NewPrincipal(a.TenantID, a.UserID, email, a.UserName, ""), func(tx pgx.Tx) error {
		q := store.New(tx)
		// The password was checked before: it is still theirs, and they are
		// still active, now that their row is held (a password changed, or
		// they deactivated, meanwhile signs nobody in).
		u, err := holdUser(ctx, q, a.UserID)
		if database.IsNoRows(err) || (err == nil && (!u.Active || u.PasswordHash == nil || *u.PasswordHash != a.PasswordHash)) {
			return apperr.New(api.ErrorCodeSIGNINFAILED, "The email or password is wrong.")
		}
		if err != nil {
			return err
		}
		if PasswordRefused(u.MicrosoftRequired, u.Role) {
			return microsoftRequired()
		}
		if next, err = m.nextStep(ctx, q, u); err != nil || next != nil {
			return err
		}
		if now, err = databaseSecond(ctx, q); err != nil {
			return err
		}
		if u.TokensValidAfter != nil && now.Before(*u.TokensValidAfter) { // never before their sessions were last ended
			now = *u.TokensValidAfter
		}
		if err := q.SignedIn(ctx, a.UserID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.signed_in", EntityType: "user", EntityID: a.UserID})
	})
	if err != nil {
		return Session{}, err
	}
	if next != nil {
		return Session{}, next
	}
	return m.session(a.TenantID, a.UserID, authn.MethodPassword, now)
}

// passwordAllowed leaves out the accounts whose company requires Microsoft
// (unless they are its owners); refused says whether any was.
func (m *Module) passwordAllowed(ctx context.Context, accounts []store.AccountsByEmailRow) ([]store.AccountsByEmailRow, bool, error) {
	var out []store.AccountsByEmailRow
	refused := false
	for _, a := range accounts {
		err := database.InTx(ctx, m.pool, authz.TenantOnly(a.TenantID), func(tx pgx.Tx) error {
			u, err := store.New(tx).UserAuth(ctx, a.UserID)
			if err == nil && PasswordRefused(u.MicrosoftRequired, u.Role) {
				refused = true
			} else if err == nil {
				out = append(out, a)
			}
			return err
		})
		if err != nil {
			return nil, false, err
		}
	}
	return out, refused, nil
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
		u, err := holdUser(ctx, q, p.UserID) // a sign-in under way finishes first, or after the change (and fails)
		if err != nil {
			return err
		}
		if PasswordRefused(u.MicrosoftRequired, u.Role) {
			return microsoftRequired()
		}
		if u.PasswordHash == nil {
			return passwordNotSet()
		}
		if !CheckPassword(*u.PasswordHash, current) {
			return apperr.Validation(apperr.FieldErrors{{Field: "currentPassword", Message: "is not your current password"}})
		}
		if problem := PasswordProblem(next, u.Email, u.Name); problem != "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "newPassword", Message: problem}})
		}
		hash, err := HashPassword(next)
		if err != nil {
			return err
		}
		now, err := sessionsEndedAt(ctx, q, u)
		if err != nil {
			return err
		}
		if err := q.SetPassword(ctx, store.SetPasswordParams{ID: u.ID, PasswordHash: &hash, ValidAfter: &now}); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "user.password_changed", EntityType: "user", EntityID: u.ID}); err != nil {
			return err
		}
		out, err = m.session(p.TenantID, p.UserID, authn.MethodPassword, now)
		return err
	})
	return out, err
}

func passwordNotSet() error {
	return apperr.New(api.ErrorCodePASSWORDNOTSET, "You sign in with Microsoft and have no HiringPlug password. To add one, use Forgot password on the sign-in page.")
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
