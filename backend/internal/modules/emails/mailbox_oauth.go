package emails

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// OAuthProvider endpoints are code-owned; browser input can choose only a provider name.
type OAuthProvider struct{ AuthorizeURL, TokenURL, ProfileURL, SendURL, Scope string }

// endpoint is the provider's code-owned endpoints (overridden only by tests).
func (s *Service) endpoint(name string) OAuthProvider {
	if p, ok := s.connections.Providers[name]; ok {
		return p
	}
	if name == "google" {
		return OAuthProvider{ //nolint:gosec // Official public OAuth endpoints/scopes, not credentials.
			AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token",
			ProfileURL: "https://www.googleapis.com/oauth2/v3/userinfo", SendURL: "https://gmail.googleapis.com/gmail/v1/users/me/messages/send",
			Scope: "openid email https://www.googleapis.com/auth/gmail.send",
		}
	}
	return OAuthProvider{ //nolint:gosec // Official public OAuth endpoints/scopes, not credentials.
		AuthorizeURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		ProfileURL: "https://graph.microsoft.com/v1.0/me?$select=mail,userPrincipalName", SendURL: "https://graph.microsoft.com/v1.0/me/sendMail",
		Scope: "openid offline_access User.Read Mail.Send Mail.ReadWrite",
	}
}
func (s *Service) callbackURL() string {
	return strings.TrimRight(s.connections.PublicURL, "/") + "/v1/mailbox/callback"
}

type oauthState struct {
	Tenant, User, Provider, Nonce string
	Expires                       int64
}

func randomMailboxToken() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
func stateHash(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}
func (s *Service) signMailboxState(claim oauthState) string {
	raw, _ := json.Marshal(claim)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, s.connections.KeysSecret)
	_, _ = mac.Write([]byte("mailbox-state:" + encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) readMailboxState(state string) (oauthState, error) {
	var claim oauthState
	parts := strings.Split(state, ".")
	if len(parts) != 2 || len(s.connections.KeysSecret) < 32 {
		return claim, errors.New("invalid mailbox state")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claim, errors.New("invalid mailbox state")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claim, errors.New("invalid mailbox state")
	}
	mac := hmac.New(sha256.New, s.connections.KeysSecret)
	_, _ = mac.Write([]byte("mailbox-state:" + parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return claim, errors.New("invalid mailbox state")
	}
	if json.Unmarshal(raw, &claim) != nil || claim.Expires <= time.Now().Unix() || claim.Tenant == "" || claim.User == "" || (claim.Provider != "google" && claim.Provider != "microsoft") {
		return claim, errors.New("expired mailbox state")
	}
	return claim, nil
}
func (s *Service) authorizeMailbox(ctx context.Context, p authz.Principal, provider string) (string, error) {
	endpoint := s.endpoint(provider)
	if len(s.connections.KeysSecret) < 32 {
		return "", apperr.Validation(apperr.FieldErrors{{Field: "provider", Message: "mailbox encryption is not configured on this server"}})
	}
	app, err := s.appIn(ctx, p, provider)
	if err != nil {
		return "", err
	}
	if app.ClientID == "" || app.ClientSecret == "" {
		return "", apperr.Validation(apperr.FieldErrors{{Field: "provider", Message: "this provider's app is not set up: an owner or admin enters it in Settings > Email connection"}})
	}
	expires := time.Now().Add(10 * time.Minute)
	state := s.signMailboxState(oauthState{Tenant: p.TenantID, User: p.UserID, Provider: provider, Nonce: randomMailboxToken(), Expires: expires.Unix()})
	verifier := randomMailboxToken()
	challenge := sha256.Sum256([]byte(verifier))
	sealed, err := s.sealMailbox(p.TenantID, p.UserID, "state:"+provider, verifier)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).LockMailboxUser(ctx); err != nil {
			return err
		}
		return store.New(tx).SaveMailboxState(ctx, store.SaveMailboxStateParams{StateHash: stateHash(state), Provider: provider, SealedVerifier: sealed, ExpiresAt: expires})
	})
	if err != nil {
		return "", err
	}
	target, _ := url.Parse(endpoint.AuthorizeURL)
	q := target.Query()
	q.Set("client_id", app.ClientID)
	q.Set("redirect_uri", s.callbackURL())
	q.Set("response_type", "code")
	q.Set("scope", endpoint.Scope)
	q.Set("state", state)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("code_challenge_method", "S256")
	if provider == "google" {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
	} else {
		q.Set("prompt", "select_account")
	}
	target.RawQuery = q.Encode()
	return target.String(), nil
}

// parkMailbox handles the provider's redirect: it checks the signed state
// and parks the sign-in code (sealed) on it, once. It returns a one-time
// ticket for the browser that was redirected. Nothing is connected yet: the
// person who started it finishes, signed in, with the ticket (finishMailbox),
// so a sign-in link passed to someone else connects nothing.
func (s *Service) parkMailbox(ctx context.Context, state, code, denied string) (string, error) {
	claim, err := s.readMailboxState(state)
	if err != nil {
		return "", err
	}
	p := authz.NewPrincipalWithAccess(claim.Tenant, claim.User, "", "", api.RoleSourcer, []api.Permission{api.PermissionEmailsSend}, authz.Scope{Users: []string{}})
	var sealedCode, ticketHash *string
	ticket := ""
	if denied == "" && code != "" {
		sealed, err := s.sealMailbox(claim.Tenant, claim.User, "code:"+claim.Provider, code)
		if err != nil {
			return "", err
		}
		ticket = randomMailboxToken()
		hash := stateHash(ticket)
		sealedCode, ticketHash = &sealed, &hash
	}
	var parked int64
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		parked, err = store.New(tx).ParkMailboxCode(ctx, store.ParkMailboxCodeParams{StateHash: stateHash(state), SealedCode: sealedCode, TicketHash: ticketHash})
		return err
	})
	if err != nil || parked != 1 {
		return "", errors.New("mailbox authorization expired; connect again")
	}
	if ticket == "" {
		return "", errors.New("mailbox authorization was not completed")
	}
	return ticket, nil
}

// finishMailbox connects the mailbox whose sign-in was parked for ticket. Only
// the person who started the connection finds it (row-level security shows
// each person their own state), so the mailbox of whoever signed in at the
// provider is connected to nobody else's account.
func (s *Service) finishMailbox(ctx context.Context, p authz.Principal, ticket string) error {
	notCompleted := apperr.Validation(apperr.FieldErrors{{Field: "ticket", Message: "the mailbox sign-in was not completed or has expired: connect again"}})
	var saved store.EmailMailboxState
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		hash := stateHash(ticket)
		saved, err = store.New(tx).ParkedMailbox(ctx, &hash)
		return err
	})
	if database.IsNoRows(err) || (err == nil && saved.SealedCode == nil) {
		return notCompleted
	}
	if err != nil {
		return err
	}
	var verifier, code string
	if err = s.openMailbox(p.TenantID, p.UserID, "state:"+saved.Provider, saved.SealedVerifier, &verifier); err != nil {
		return err
	}
	if err = s.openMailbox(p.TenantID, p.UserID, "code:"+saved.Provider, *saved.SealedCode, &code); err != nil {
		return err
	}
	endpoint := s.endpoint(saved.Provider)
	app, err := s.appIn(ctx, p, saved.Provider)
	if err != nil {
		return err
	}
	tokens, err := s.exchangeMailboxToken(ctx, endpoint, app, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.callbackURL()}, "code_verifier": {verifier}})
	if err != nil {
		return notCompleted
	}
	address, err := s.mailboxProfile(ctx, saved.Provider, endpoint, tokens.AccessToken)
	if err != nil {
		return notCompleted
	}
	sealed, err := s.sealMailbox(p.TenantID, p.UserID, saved.Provider, tokens)
	if err != nil {
		return err
	}
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).LockMailboxUser(ctx); err != nil {
			return err
		}
		claimed, err := store.New(tx).FinishMailboxState(ctx, saved.StateHash)
		if err != nil {
			return err
		}
		if claimed != 1 {
			return notCompleted // cancelled or replaced meanwhile
		}
		if _, err := store.New(tx).MailboxForUser(ctx, p.UserID); err != nil && !database.IsNoRows(err) {
			return err
		}
		_, err = store.New(tx).SaveMailbox(ctx, store.SaveMailboxParams{Provider: saved.Provider, Address: address, SealedSecret: sealed})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "mailbox.connected", EntityType: "user", EntityID: p.UserID, Changes: map[string]any{"provider": saved.Provider}})
	})
}
