package identity

// Two-step sign-in: after the password, a code from an authenticator app (or
// a single-use recovery code). The app's secret is sealed with a key derived
// from the sign-in secret; a code is accepted once. An owner can require it
// of everyone: those without it set it up at their next sign-in, before they
// get a session. A password reset link never skips it.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
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

// UseTwoStepKey derives the keys that seal authenticator secrets and sign
// challenges from the sign-in secret (HP_AUTH_JWT_SECRET). Changing that
// secret signs everyone out and means setting the app up again.
func (m *Module) UseTwoStepKey(secret []byte) {
	seal := sha256.Sum256(append([]byte("hiringplug two-step seal v1:"), secret...))
	challenge := sha256.Sum256(append([]byte("hiringplug two-step challenge v1:"), secret...))
	m.twoStep = twoStepKeys{seal: seal[:], challenge: challenge[:]}
}

type twoStepKeys struct{ seal, challenge []byte }

// TwoStepStatus is a person's two-step sign-in.
type TwoStepStatus struct {
	Enabled           bool
	EnabledAt         *time.Time
	RecoveryCodesLeft int
	Required          bool
}

// TwoStepSetup is an authenticator secret being added.
type TwoStepSetup struct{ Secret, URI string }

var errTwoStepOff = errors.New("two-step sign-in is not configured")

func (m *Module) aead() (cipher.AEAD, error) {
	if len(m.twoStep.seal) == 0 {
		return nil, errTwoStepOff
	}
	block, err := aes.NewCipher(m.twoStep.seal)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (m *Module) sealSecret(tenantID, userID, secret string) (string, error) {
	g, err := m.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(secret), []byte("two-step:"+tenantID+":"+userID))), nil
}

func (m *Module) openSecret(tenantID, userID string, sealed *string) (string, error) {
	g, err := m.aead()
	if err != nil {
		return "", err
	}
	if sealed == nil {
		return "", errors.New("no authenticator secret")
	}
	raw, err := base64.RawURLEncoding.DecodeString(*sealed)
	if err != nil || len(raw) < g.NonceSize() {
		return "", errors.New("authenticator secret unreadable")
	}
	plain, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], []byte("two-step:"+tenantID+":"+userID))
	return string(plain), err
}

func signInExpired() error {
	return apperr.New(api.ErrorCodeSIGNINFAILED, "The sign-in has expired. Sign in again.")
}

func wrongCode() error {
	return apperr.New(api.ErrorCodeSIGNINFAILED, "The code is wrong, or was used already.")
}

// nextStep is what follows a right password: a session, or a challenge for
// the code (or to set the app up first, when the company requires it).
func (m *Module) nextStep(ctx context.Context, q *store.Queries, tenantID, userID string) (*apperr.Error, error) {
	ts, err := q.GetTwoStep(ctx, userID)
	if err != nil && !database.IsNoRows(err) {
		return nil, err
	}
	if err == nil && ts.EnabledAt != nil {
		return m.challengeError(api.ErrorCodeTWOSTEPREQUIRED, "Enter the code from your authenticator app.", tenantID, userID, false)
	}
	required, err := q.TwoStepRequired(ctx)
	if err != nil || !required {
		return nil, err
	}
	return m.challengeError(api.ErrorCodeTWOSTEPSETUPREQUIRED, "Your company requires two-step sign-in: add an authenticator app.", tenantID, userID, true)
}

func (m *Module) challengeError(code api.ErrorCode, message, tenantID, userID string, setup bool) (*apperr.Error, error) {
	if len(m.twoStep.challenge) == 0 {
		return nil, errTwoStepOff
	}
	token := SignChallenge(m.twoStep.challenge, Challenge{TenantID: tenantID, UserID: userID, Setup: setup, Expires: time.Now().Add(challengeLife).Unix()})
	return apperr.New(code, message).WithDetail("challenge", token), nil
}

// withChallenge runs fn for the person a challenge names, in their company,
// once the attempt is allowed.
func (m *Module) withChallenge(ctx context.Context, clientAddr, token string, setup bool, fn func(tx pgx.Tx, q *store.Queries, u store.UserAuthRow) error) error {
	c, err := ReadChallenge(m.twoStep.challenge, token, time.Now())
	if err != nil || c.Setup != setup {
		return signInExpired()
	}
	if !m.limits.twoStep.Allow(clientAddr) || !m.limits.twoStepUser.Allow(c.UserID) {
		return tooMany()
	}
	return database.InTx(ctx, m.pool, authz.System(c.TenantID), func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, c.UserID)
		if database.IsNoRows(err) || (err == nil && !u.Active) {
			return signInExpired()
		}
		if err != nil {
			return err
		}
		if setup { // a password alone never replaces an app already on
			if ts, err := q.GetTwoStep(ctx, u.ID); err == nil && ts.EnabledAt != nil {
				return signInExpired()
			} else if err != nil && !database.IsNoRows(err) {
				return err
			}
		}
		return fn(tx, q, u)
	})
}

// checkCode accepts an app code (once, near its time) or, if allowRecovery,
// an unused recovery code; how says which.
func (m *Module) checkCode(ctx context.Context, q *store.Queries, tenantID, userID, code string, allowRecovery bool) (how string, err error) {
	if allowRecovery && LooksLikeRecoveryCode(code) {
		n, err := q.UseRecoveryCode(ctx, store.UseRecoveryCodeParams{UserID: userID, Hash: HashRecoveryCode(code)})
		if err != nil || n == 0 {
			return "", firstErr(err, wrongCode())
		}
		return "recovery_code", nil
	}
	ts, err := q.GetTwoStep(ctx, userID)
	if database.IsNoRows(err) || (err == nil && ts.EnabledAt == nil) {
		return "", wrongCode()
	}
	if err != nil {
		return "", err
	}
	secret, err := m.openSecret(tenantID, userID, ts.SecretSealed)
	if err != nil {
		return "", err
	}
	step, ok := CheckTOTP(secret, code, time.Now(), ts.LastStep)
	if !ok {
		return "", wrongCode()
	}
	if n, err := q.UseTwoStepStep(ctx, store.UseTwoStepStepParams{UserID: userID, Step: step}); err != nil || n == 0 {
		return "", firstErr(err, wrongCode()) // used at the same moment by another sign-in
	}
	return "app", nil
}

// signedIn records a password sign-in and mints its session (never before
// the user's sessions were last ended; refused once the company requires
// Microsoft, unless they are an owner).
func (m *Module) signedIn(ctx context.Context, tx pgx.Tx, q *store.Queries, u store.UserAuthRow, how string) (Session, error) {
	if PasswordRefused(u.MicrosoftRequired, u.Role) {
		return Session{}, microsoftRequired()
	}
	if err := q.SignedIn(ctx, u.ID); err != nil {
		return Session{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: "user.signed_in", EntityType: "user", EntityID: u.ID, Changes: map[string]any{"twoStep": how}}); err != nil {
		return Session{}, err
	}
	now := time.Now().Truncate(time.Second)
	if u.TokensValidAfter != nil && now.Before(*u.TokensValidAfter) {
		now = *u.TokensValidAfter
	}
	return m.session(u.TenantID, u.ID, authn.MethodPassword, now)
}

// SignInTwoStep finishes a sign-in with the app's code or a recovery code.
func (m *Module) SignInTwoStep(ctx context.Context, clientAddr, challenge, code string) (Session, error) {
	var out Session
	err := m.withChallenge(ctx, clientAddr, challenge, false, func(tx pgx.Tx, q *store.Queries, u store.UserAuthRow) error {
		how, err := m.checkCode(ctx, q, u.TenantID, u.ID, code, true)
		if err != nil {
			return err
		}
		out, err = m.signedIn(ctx, tx, q, u, how)
		return err
	})
	return out, err
}

// newSetup makes a new secret for the person, kept pending until a code
// confirms it.
func (m *Module) newSetup(ctx context.Context, q *store.Queries, tenantID, userID, email string) (TwoStepSetup, error) {
	secret := NewTOTPSecret()
	sealed, err := m.sealSecret(tenantID, userID, secret)
	if err != nil {
		return TwoStepSetup{}, err
	}
	if err := q.SetTwoStepPending(ctx, store.SetTwoStepPendingParams{UserID: userID, PendingSealed: &sealed}); err != nil {
		return TwoStepSetup{}, err
	}
	t, err := q.GetTenant(ctx, tenantID)
	if err != nil {
		return TwoStepSetup{}, err
	}
	return TwoStepSetup{Secret: secret, URI: TOTPURI(secret, t.Name, email)}, nil
}

// enable confirms the pending secret with its first code and makes new
// recovery codes.
func (m *Module) enable(ctx context.Context, tx pgx.Tx, q *store.Queries, tenantID, userID, code string) ([]string, error) {
	ts, err := q.GetTwoStep(ctx, userID)
	if database.IsNoRows(err) || (err == nil && ts.PendingSealed == nil) {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "code", Message: "start adding the app again"}})
	}
	if err != nil {
		return nil, err
	}
	secret, err := m.openSecret(tenantID, userID, ts.PendingSealed)
	if err != nil {
		return nil, err
	}
	step, ok := CheckTOTP(secret, code, time.Now(), 0)
	if !ok {
		return nil, apperr.Validation(apperr.FieldErrors{{Field: "code", Message: "is not the code the app shows now"}})
	}
	codes, hashes := NewRecoveryCodes()
	if _, err := q.EnableTwoStep(ctx, store.EnableTwoStepParams{UserID: userID, LastStep: step, RecoveryHashes: hashes}); err != nil {
		return nil, err
	}
	return codes, audit.Record(ctx, tx, audit.Entry{Action: "user.two_step_enabled", EntityType: "user", EntityID: userID})
}

// StartTwoStepAtSignIn adds an app while signing in (the company requires it).
func (m *Module) StartTwoStepAtSignIn(ctx context.Context, clientAddr, challenge string) (TwoStepSetup, error) {
	var out TwoStepSetup
	err := m.withChallenge(ctx, clientAddr, challenge, true, func(_ pgx.Tx, q *store.Queries, u store.UserAuthRow) error {
		var err error
		out, err = m.newSetup(ctx, q, u.TenantID, u.ID, u.Email)
		return err
	})
	return out, err
}

// ConfirmTwoStepAtSignIn turns two-step sign-in on with the app's first code
// and signs the person in.
func (m *Module) ConfirmTwoStepAtSignIn(ctx context.Context, clientAddr, challenge, code string) (Session, []string, error) {
	var out Session
	var codes []string
	err := m.withChallenge(ctx, clientAddr, challenge, true, func(tx pgx.Tx, q *store.Queries, u store.UserAuthRow) error {
		var err error
		if codes, err = m.enable(ctx, tx, q, u.TenantID, u.ID, code); err != nil {
			return err
		}
		out, err = m.signedIn(ctx, tx, q, u, "app")
		return err
	})
	return out, codes, err
}

// MyTwoStep is the signed-in person's two-step sign-in.
func (m *Module) MyTwoStep(ctx context.Context, p authz.Principal) (TwoStepStatus, error) {
	var out TwoStepStatus
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = twoStepStatus(ctx, store.New(tx), p.UserID)
		return err
	})
	return out, err
}

func twoStepStatus(ctx context.Context, q *store.Queries, userID string) (TwoStepStatus, error) {
	var out TwoStepStatus
	required, err := q.TwoStepRequired(ctx)
	if err != nil {
		return out, err
	}
	out.Required = required
	ts, err := q.GetTwoStep(ctx, userID)
	if database.IsNoRows(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Enabled, out.EnabledAt, out.RecoveryCodesLeft = ts.EnabledAt != nil, ts.EnabledAt, len(ts.RecoveryHashes)
	return out, nil
}

// StartMyTwoStep starts adding an app (an app added before keeps working
// until the new one is confirmed).
func (m *Module) StartMyTwoStep(ctx context.Context, p authz.Principal) (TwoStepSetup, error) {
	var out TwoStepSetup
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = m.newSetup(ctx, store.New(tx), p.TenantID, p.UserID, p.Email)
		return err
	})
	return out, err
}

// ConfirmMyTwoStep turns two-step sign-in on (or moves it to a new app).
func (m *Module) ConfirmMyTwoStep(ctx context.Context, p authz.Principal, code string) ([]string, error) {
	if !m.limits.twoStepUser.Allow(p.UserID) {
		return nil, tooMany()
	}
	var codes []string
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		var err error
		codes, err = m.enable(ctx, tx, store.New(tx), p.TenantID, p.UserID, code)
		return err
	})
	return codes, err
}

// NewRecoveryCodes replaces the recovery codes, with a code from the app.
func (m *Module) NewRecoveryCodes(ctx context.Context, p authz.Principal, code string) ([]string, error) {
	if !m.limits.twoStepUser.Allow(p.UserID) {
		return nil, tooMany()
	}
	var codes []string
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := m.checkCode(ctx, q, p.TenantID, p.UserID, code, false); err != nil {
			return apperr.Validation(apperr.FieldErrors{{Field: "code", Message: "is not the code the app shows now"}})
		}
		var hashes []string
		codes, hashes = NewRecoveryCodes()
		if err := q.SetRecoveryCodes(ctx, store.SetRecoveryCodesParams{UserID: p.UserID, RecoveryHashes: hashes}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.two_step_recovery_codes", EntityType: "user", EntityID: p.UserID})
	})
	return codes, err
}

// DisableMyTwoStep turns it off with the password (or, for someone who signs
// in with Microsoft and has none, the app's code), unless the company
// requires it.
func (m *Module) DisableMyTwoStep(ctx context.Context, p authz.Principal, password, code *string) (TwoStepStatus, error) {
	if !m.limits.change.Allow(p.UserID) {
		return TwoStepStatus{}, tooMany()
	}
	var out TwoStepStatus
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, p.UserID)
		if err != nil {
			return err
		}
		if err := m.provePerson(ctx, q, u, password, code); err != nil {
			return err
		}
		if required, err := q.TwoStepRequired(ctx); err != nil || required {
			return firstErr(err, teamProblem("Your company requires two-step sign-in; only an owner can stop requiring it."))
		}
		if _, err := q.DisableTwoStep(ctx, p.UserID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "user.two_step_disabled", EntityType: "user", EntityID: p.UserID}); err != nil {
			return err
		}
		out, err = twoStepStatus(ctx, q, p.UserID)
		return err
	})
	return out, err
}

// ResetTwoStep removes someone's app (a lost phone) and signs them out; only
// owners reset owners.
func (m *Module) ResetTwoStep(ctx context.Context, p authz.Principal, userID string) error {
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("team member")
		}
		if err != nil {
			return err
		}
		if u.Role == string(api.RoleOwner) && p.Role != api.RoleOwner {
			return teamProblem("Only an owner can reset an owner's two-step sign-in.")
		}
		if _, err := q.DisableTwoStep(ctx, userID); err != nil {
			return err
		}
		if err := q.EndUserSessions(ctx, userID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.two_step_reset", EntityType: "user", EntityID: userID})
	})
}

// SetTwoStepRequired requires two-step sign-in of everyone (signing out those
// without it), or stops requiring it. The owner needs it first.
func (m *Module) SetTwoStepRequired(ctx context.Context, p authz.Principal, required bool) error {
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if required {
			mine, err := twoStepStatus(ctx, q, p.UserID)
			if err != nil {
				return err
			}
			if !mine.Enabled {
				return apperr.New(api.ErrorCodeTWOSTEPNOTENABLED, "Set up two-step sign-in for yourself first (Your account).")
			}
		}
		if err := q.SetTwoStepRequired(ctx, required); err != nil {
			return err
		}
		if required {
			if err := q.EndSessionsWithoutTwoStep(ctx); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "agency.two_step_required", EntityType: "tenant", EntityID: p.TenantID, Changes: map[string]any{"required": required}})
	})
}

// TwoStepUsers is who has two-step sign-in on (for the team list).
func (m *Module) TwoStepUsers(ctx context.Context, p authz.Principal) (map[string]bool, error) {
	out := map[string]bool{}
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		ids, err := store.New(tx).TwoStepUsers(ctx)
		for _, id := range ids {
			out[id] = true
		}
		return err
	})
	return out, err
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
