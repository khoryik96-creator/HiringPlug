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

func (s *Service) provider(name string) (OAuthProvider, OAuthApplication) {
	if p, ok := s.connections.Providers[name]; ok {
		if name == "google" {
			return p, s.connections.Google
		}
		return p, s.connections.Microsoft
	}
	if name == "google" {
		return OAuthProvider{ //nolint:gosec // Official public OAuth endpoints/scopes, not credentials.
			AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token",
			ProfileURL: "https://www.googleapis.com/oauth2/v3/userinfo", SendURL: "https://gmail.googleapis.com/gmail/v1/users/me/messages/send",
			Scope: "openid email https://www.googleapis.com/auth/gmail.send",
		}, s.connections.Google
	}
	return OAuthProvider{ //nolint:gosec // Official public OAuth endpoints/scopes, not credentials.
		AuthorizeURL: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		ProfileURL: "https://graph.microsoft.com/v1.0/me?$select=mail,userPrincipalName", SendURL: "https://graph.microsoft.com/v1.0/me/sendMail",
		Scope: "openid offline_access User.Read Mail.Send Mail.ReadWrite",
	}, s.connections.Microsoft
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
	endpoint, app := s.provider(provider)
	if len(s.connections.KeysSecret) < 32 || app.ClientID == "" || app.ClientSecret == "" {
		return "", apperr.Validation(apperr.FieldErrors{{Field: "provider", Message: "the server's application registration is not configured for this provider"}})
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
func (s *Service) completeMailbox(ctx context.Context, state, code, denied string) error {
	claim, err := s.readMailboxState(state)
	if err != nil {
		return err
	}
	p := authz.NewPrincipalWithAccess(claim.Tenant, claim.User, "", "", api.RoleSourcer, []api.Permission{api.PermissionEmailsSend}, authz.Scope{Users: []string{}})
	var saved store.EmailMailboxState
	// Claim once; replacement or disconnect invalidates this lease even in flight.
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		saved, err = store.New(tx).ConsumeMailboxState(ctx, stateHash(state))
		return err
	})
	if err != nil {
		return errors.New("mailbox authorization expired; connect again")
	}
	if denied != "" || code == "" || saved.Provider != claim.Provider {
		return errors.New("mailbox authorization was not completed")
	}
	var verifier string
	if err = s.openMailbox(claim.Tenant, claim.User, "state:"+claim.Provider, saved.SealedVerifier, &verifier); err != nil {
		return err
	}
	endpoint, app := s.provider(claim.Provider)
	tokens, err := s.exchangeMailboxToken(ctx, endpoint, app, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.callbackURL()}, "code_verifier": {verifier}})
	if err != nil {
		return err
	}
	address, err := s.mailboxProfile(ctx, claim.Provider, endpoint, tokens.AccessToken)
	if err != nil {
		return err
	}
	sealed, err := s.sealMailbox(claim.Tenant, claim.User, claim.Provider, tokens)
	if err != nil {
		return err
	}
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := store.New(tx).LockMailboxUser(ctx); err != nil {
			return err
		}
		claimed, err := store.New(tx).FinishMailboxState(ctx, stateHash(state))
		if err != nil {
			return err
		}
		if claimed != 1 {
			return errors.New("mailbox authorization was cancelled or replaced")
		}
		// Lock before replacement, including when another connection already exists.
		if _, err := store.New(tx).MailboxForUser(ctx, claim.User); err != nil && !database.IsNoRows(err) {
			return err
		}
		_, err = store.New(tx).SaveMailbox(ctx, store.SaveMailboxParams{Provider: claim.Provider, Address: address, SealedSecret: sealed})
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "mailbox.connected", EntityType: "user", EntityID: claim.User, Changes: map[string]any{"provider": claim.Provider}})
	})
}
