package identity

// Sign in with Microsoft (Entra ID). Only people already on the team
// (active, or invited: signing in accepts the invitation) of a company that
// turned it on, with an account of the company's own Microsoft organisation;
// never personal accounts. The first sign-in links the account to the team
// member with its address; later ones match the account (organisation and
// object ID), whatever its address has become. Microsoft sign-ins skip the
// two-step code; sessions say how they were signed in (amr), so a company
// that requires Microsoft refuses password sessions to everyone but owners.
//
// The flow: start (here) saves the sign-in, its PKCE verifier and nonce
// sealed, with the SHA-256 of a value the starting browser tab keeps; the
// callback (here) parks Microsoft's code on it once and sends the browser
// back to the app with a one-time ticket; complete (microsoft_complete.go)
// takes the ticket with the tab's value, exchanges the code, checks the ID
// token fully (platform/entra) and signs in.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// MicrosoftApp is a Microsoft app registration people sign in through.
type MicrosoftApp struct {
	ClientID, ClientSecret string
	Agency                 bool // the company's own (Settings > Email connection), not the server's
}

func (a MicrosoftApp) usable() bool { return a.ClientID != "" && a.ClientSecret != "" }

// MicrosoftApps finds the apps (the emails module keeps them).
type MicrosoftApps interface {
	// MicrosoftSignInApp is the company's own app, or else the server's, in
	// the caller's transaction (scoped to the company).
	MicrosoftSignInApp(ctx context.Context, tx pgx.Tx, tenantID string) (MicrosoftApp, error)
	// ServerMicrosoftApp is the server's app (HP_MAIL_MICROSOFT_*), for
	// sign-ins that name no company.
	ServerMicrosoftApp() MicrosoftApp
}

// MicrosoftSettings configures signing in with Microsoft.
type MicrosoftSettings struct {
	Apps       MicrosoftApps
	Secret     []byte       // HP_AUTH_JWT_SECRET: seals sign-ins under way
	Login      string       // Microsoft's sign-in service; tests only
	HTTPClient *http.Client // tests only
}

type microsoftSignIn struct {
	apps  MicrosoftApps
	entra *entra.Client
	seal  cipher.AEAD
}

// UseMicrosoft turns signing in with Microsoft on for the server (each
// company still turns it on for itself).
func (m *Module) UseMicrosoft(s MicrosoftSettings) {
	key := sha256.Sum256(append([]byte("hiringplug microsoft sign-in seal v1:"), s.Secret...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(err) // a 32-byte key always makes a cipher
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	m.ms = &microsoftSignIn{apps: s.Apps, entra: entra.New(s.Login, s.HTTPClient), seal: g}
}

// UseMicrosoftService points signing in with Microsoft at another sign-in
// service: a fake one, in tests (UseMicrosoft first).
func (m *Module) UseMicrosoftService(login string, httpClient *http.Client) {
	m.ms.entra = entra.New(login, httpClient)
}

// microsoftCallback is the redirect URI to register with the app.
func (m *Module) microsoftCallback() string { return m.publicURL + "/v1/auth/microsoft/callback" }

// How long a sign-in may take, and a company choice after it.
const (
	microsoftFlowLife   = 10 * time.Minute
	microsoftChoiceLife = 5 * time.Minute
)

func (s *microsoftSignIn) sealJSON(aad string, v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, s.seal.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(s.seal.Seal(nonce, nonce, raw, []byte(aad))), nil
}

func (s *microsoftSignIn) openJSON(aad, sealed string, v any) error {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(raw) < s.seal.NonceSize() {
		return errors.New("microsoft sign-in: unreadable")
	}
	plain, err := s.seal.Open(nil, raw[:s.seal.NonceSize()], raw[s.seal.NonceSize():], []byte(aad))
	if err != nil {
		return errors.New("microsoft sign-in: unreadable")
	}
	return json.Unmarshal(plain, v)
}

// randomToken is 32 random bytes, base64url.
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the system's random source failing is not recoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// flowSecret is what the start keeps sealed for the end.
type flowSecret struct{ Verifier, Nonce string }

func microsoftUnavailable() error {
	return apperr.New(api.ErrorCodeMICROSOFTSIGNINUNAVAILABLE, "Signing in with Microsoft is not set up here. Sign in with your email and password, or ask an owner.")
}

// MicrosoftStart is what starting a sign-in may name.
type MicrosoftStart struct {
	BindingHash          string // SHA-256 (hex) of the browser tab's value
	Email, Company, Link string // all optional: see routeMicrosoft
}

// StartMicrosoft saves a new sign-in and returns Microsoft's address to send
// the browser to.
func (m *Module) StartMicrosoft(ctx context.Context, clientAddr string, in MicrosoftStart) (string, error) {
	email := normEmail(in.Email)
	if !m.limits.msStart.Allow(clientAddr) || (email != "" && !m.limits.signInEmail.Allow(email)) {
		return "", tooMany()
	}
	if m.ms == nil {
		return "", microsoftUnavailable()
	}
	route, err := m.routeMicrosoft(ctx, email, in.Company, in.Link)
	if err != nil {
		return "", err
	}
	state, verifier, nonce := randomToken(), randomToken(), randomToken()
	key := hashLink(state)
	sealed, err := m.ms.sealJSON("flow:"+key, flowSecret{Verifier: verifier, Nonce: nonce})
	if err != nil {
		return "", err
	}
	target, err := m.ms.entra.AuthorizeURL(entra.Authorize{
		Authority: route.authority, ClientID: route.app.ClientID, RedirectURI: m.microsoftCallback(),
		State: state, Nonce: nonce, Challenge: pkceChallenge(verifier), LoginHint: email,
	})
	if err != nil {
		return "", err
	}
	var tenant *string
	if route.tenantID != "" {
		tenant = &route.tenantID
	}
	err = store.New(m.pool).SaveMicrosoftFlow(ctx, store.SaveMicrosoftFlowParams{
		KeyHash: key, BindingHash: in.BindingHash, TenantID: tenant, ClientID: route.app.ClientID, Authority: route.authority,
		Sealed: sealed, ExpiresAt: time.Now().Add(microsoftFlowLife),
	})
	return target, err
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// msRoute is where a sign-in goes: a company (its organisation, and its app
// or the server's), or (tenantID "") any company, through the server's app.
type msRoute struct {
	tenantID, authority string
	app                 MicrosoftApp
}

// routeMicrosoft finds the company a sign-in is for: the invitation's, the
// careers page address's, the one company that signs in with Microsoft
// where the email is on the team, the only company on the server that signs
// in with Microsoft; else any company, through the server's app (the
// account's organisation then finds it). An email not on any such team is
// routed as if none were typed.
func (m *Module) routeMicrosoft(ctx context.Context, email, company, link string) (msRoute, error) {
	q := store.New(m.pool)
	switch {
	case link != "":
		l, err := q.LinkByHash(ctx, hashLink(link))
		if database.IsNoRows(err) || (err == nil && l.Kind != "invite") {
			return msRoute{}, linkGone()
		}
		if err != nil {
			return msRoute{}, err
		}
		return m.tenantRoute(ctx, l.TenantID)
	case company != "":
		rows, err := q.MicrosoftSignInTenants(ctx, &company)
		if err != nil || len(rows) == 0 {
			return msRoute{}, firstErr(err, microsoftUnavailable())
		}
		return m.tenantRoute(ctx, rows[0].TenantID)
	}
	if email != "" {
		ids, err := q.MicrosoftTenantsByEmail(ctx, email)
		if err != nil {
			return msRoute{}, err
		}
		if len(ids) == 1 {
			return m.tenantRoute(ctx, ids[0])
		}
	}
	all, err := q.MicrosoftSignInTenants(ctx, nil)
	if err != nil {
		return msRoute{}, err
	}
	if len(all) == 1 {
		return m.tenantRoute(ctx, all[0].TenantID)
	}
	if app := m.ms.apps.ServerMicrosoftApp(); app.usable() {
		return msRoute{authority: entra.Organizations, app: app}, nil
	}
	return msRoute{}, microsoftUnavailable()
}

// tenantRoute is a company's route, if it signs in with Microsoft and has an app.
func (m *Module) tenantRoute(ctx context.Context, tenantID string) (msRoute, error) {
	var out msRoute
	err := database.InTx(ctx, m.pool, authz.System(tenantID), func(tx pgx.Tx) error {
		org, app, err := m.tenantMicrosoft(ctx, tx, store.New(tx), tenantID)
		out = msRoute{tenantID: tenantID, authority: org, app: app}
		return err
	})
	return out, err
}

// tenantMicrosoft is a company's organisation and app, in its transaction;
// unavailable when it does not sign in with Microsoft or has no app.
func (m *Module) tenantMicrosoft(ctx context.Context, tx pgx.Tx, q *store.Queries, tenantID string) (string, MicrosoftApp, error) {
	s, err := q.MicrosoftSettings(ctx)
	if err != nil {
		return "", MicrosoftApp{}, err
	}
	if !s.MicrosoftSignIn || s.MicrosoftTenantID == nil {
		return "", MicrosoftApp{}, microsoftUnavailable()
	}
	app, err := m.ms.apps.MicrosoftSignInApp(ctx, tx, tenantID)
	if err != nil {
		return "", MicrosoftApp{}, err
	}
	if !app.usable() {
		return "", MicrosoftApp{}, microsoftUnavailable()
	}
	return *s.MicrosoftTenantID, app, nil
}

// MicrosoftCallback handles Microsoft's redirect: it parks the code on the
// sign-in its state names, once, and returns where to send the browser: the
// app, with a one-time ticket, or with error=cancelled or error=failed.
// Nothing Microsoft sent is ever passed on.
func (m *Module) MicrosoftCallback(ctx context.Context, clientAddr, state, code, providerError string) string {
	back := m.publicURL + "/#/sign-in/microsoft?"
	switch {
	case providerError == "access_denied":
		return back + "error=cancelled"
	case providerError != "" || state == "" || code == "" || m.ms == nil:
		return back + "error=failed"
	case !m.limits.msCallback.Allow(clientAddr):
		return back + "error=failed"
	}
	key := hashLink(state)
	sealed, err := m.ms.sealJSON("code:"+key, code)
	if err != nil {
		return back + "error=failed"
	}
	ticket := randomToken()
	parked, err := store.New(m.pool).ParkMicrosoftFlow(ctx, store.ParkMicrosoftFlowParams{KeyHash: key, SealedCode: sealed, TicketHash: hashLink(ticket)})
	if err != nil {
		logx.From(ctx).Warn("microsoft sign-in: the code could not be kept", "error", err)
	}
	if err != nil || parked != 1 {
		return back + "error=failed"
	}
	return back + "ticket=" + ticket
}
