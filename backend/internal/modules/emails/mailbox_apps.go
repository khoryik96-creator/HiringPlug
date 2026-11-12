package emails

// The Microsoft and Google apps people connect their mailbox through: the
// agency's own details, entered in Settings > Email connection (the secret
// sealed with HP_MAIL_KEYS_SECRET, only its last four characters ever shown
// again), or else the server's HP_MAIL_* settings.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// appUser is the seal's "user" for an agency's app secret.
const appUser = "agency-app"

// MailApp is one provider's app as Settings shows it: never its secret.
type MailApp struct {
	Source       string // agency, server or none
	ClientID     *string
	SecretEnding *string
	UpdatedAt    *time.Time
}

// MailApps is both providers' apps, and what Settings needs to set them up.
type MailApps struct {
	StorageReady      bool
	CallbackURL       string
	Microsoft, Google MailApp
}

func (s *Service) serverApp(provider string) OAuthApplication {
	if provider == "google" {
		return s.connections.Google
	}
	return s.connections.Microsoft
}

// appFor is the app to connect and refresh mailboxes through: the agency's
// when it entered one, else the server's (empty when neither).
func (s *Service) appFor(ctx context.Context, tx pgx.Tx, tenant, provider string) (OAuthApplication, error) {
	row, err := store.New(tx).MailApp(ctx, provider)
	if database.IsNoRows(err) {
		return s.serverApp(provider), nil
	}
	if err != nil {
		return OAuthApplication{}, err
	}
	var secret string
	if err := s.openMailbox(tenant, appUser, provider, row.SealedSecret, &secret); err != nil {
		return OAuthApplication{}, err
	}
	return OAuthApplication{ClientID: row.ClientID, ClientSecret: secret}, nil
}

// appIn is appFor in a transaction of its own.
func (s *Service) appIn(ctx context.Context, p authz.Principal, provider string) (OAuthApplication, error) {
	var app OAuthApplication
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		app, err = s.appFor(ctx, tx, p.TenantID, provider)
		return err
	})
	return app, err
}

// MailApps returns the agency's apps, secrets masked.
func (s *Service) MailApps(ctx context.Context, p authz.Principal) (MailApps, error) {
	var out MailApps
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = s.mailAppsIn(ctx, tx)
		return err
	})
	return out, err
}

func (s *Service) mailAppsIn(ctx context.Context, tx pgx.Tx) (MailApps, error) {
	out := MailApps{StorageReady: len(s.connections.KeysSecret) >= 32, CallbackURL: s.callbackURL()}
	rows, err := store.New(tx).MailApps(ctx)
	if err != nil {
		return out, err
	}
	saved := map[string]store.EmailMailApp{}
	for _, r := range rows {
		saved[r.Provider] = r
	}
	describe := func(provider string) MailApp {
		if r, ok := saved[provider]; ok {
			return MailApp{Source: "agency", ClientID: &r.ClientID, SecretEnding: &r.SecretEnding, UpdatedAt: &r.UpdatedAt}
		}
		if app := s.serverApp(provider); app.ClientID != "" && app.ClientSecret != "" {
			id := app.ClientID
			return MailApp{Source: "server", ClientID: &id}
		}
		return MailApp{Source: "none"}
	}
	out.Microsoft, out.Google = describe("microsoft"), describe("google")
	return out, nil
}

// SaveMailApp saves the agency's app for a provider; a secret left out keeps
// the saved one.
func (s *Service) SaveMailApp(ctx context.Context, p authz.Principal, provider, clientID string, clientSecret *string) (MailApps, error) {
	clientID = strings.TrimSpace(clientID)
	var errs apperr.FieldErrors
	if clientID == "" || strings.ContainsAny(clientID, " \t\r\n") {
		errs.Add("clientId", "enter the app's client ID, without spaces")
	}
	if len(s.connections.KeysSecret) < 32 {
		errs.Add("clientSecret", "this server cannot store secrets yet: HP_MAIL_KEYS_SECRET must be set on the server first")
	}
	if err := errs.Err(); err != nil {
		return MailApps{}, err
	}
	var out MailApps
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		secret := ""
		if clientSecret != nil {
			secret = strings.TrimSpace(*clientSecret)
		}
		ending := ""
		var sealed string
		if secret == "" {
			old, err := q.MailApp(ctx, provider)
			if database.IsNoRows(err) {
				return apperr.Validation(apperr.FieldErrors{{Field: "clientSecret", Message: "enter the app's client secret"}})
			}
			if err != nil {
				return err
			}
			sealed, ending = old.SealedSecret, old.SecretEnding
		} else {
			var err error
			if sealed, err = s.sealMailbox(p.TenantID, appUser, provider, secret); err != nil {
				return err
			}
			ending = secret[max(0, len(secret)-4):]
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "mailbox.app_saved", EntityType: "mail_app", EntityID: provider,
			Changes: map[string]any{"clientId": clientID, "secretChanged": secret != ""}}); err != nil {
			return err
		}
		if err := q.SaveMailApp(ctx, store.SaveMailAppParams{Provider: provider, ClientID: clientID, SealedSecret: sealed, SecretEnding: ending}); err != nil {
			return err
		}
		var err error
		out, err = s.mailAppsIn(ctx, tx)
		return err
	})
	return out, err
}

// DeleteMailApp removes the agency's app for a provider (the server's is used again, if any).
func (s *Service) DeleteMailApp(ctx context.Context, p authz.Principal, provider string) (MailApps, error) {
	var out MailApps
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		if err := audit.Record(ctx, tx, audit.Entry{Action: "mailbox.app_removed", EntityType: "mail_app", EntityID: provider}); err != nil {
			return err
		}
		if err := store.New(tx).DeleteMailApp(ctx, provider); err != nil {
			return err
		}
		var err error
		out, err = s.mailAppsIn(ctx, tx)
		return err
	})
	return out, err
}
