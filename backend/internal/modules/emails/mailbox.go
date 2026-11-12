package emails

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/mail"
)

// OAuthApplication contains deployment-owned registration, never mailbox credentials.
type OAuthApplication struct{ ClientID, ClientSecret string }

// ConnectionSettings configures provider-neutral mailbox connections.
type ConnectionSettings struct {
	KeysSecret            []byte
	PublicURL, ServerMode string
	Microsoft, Google     OAuthApplication
	AllowPrivate          bool // Development tests only; production config forbids this.
	HTTPClient            *http.Client
	// Provider endpoints default to official services; overrides are for in-process tests.
	Providers map[string]OAuthProvider
}

func (m *Module) ConnectMailboxesWith(settings ConnectionSettings) {
	if settings.HTTPClient == nil {
		settings.HTTPClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	m.service.connections = settings
}

func (s *Service) mailboxStatus(ctx context.Context, p authz.Principal) (api.MailboxStatus, error) {
	out := api.MailboxStatus{StorageReady: len(s.connections.KeysSecret) >= 32, ServerMode: api.MailboxStatusServerMode(s.serverMode())}
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		apps, err := s.mailAppsIn(ctx, tx) // the agency's apps, or the server's
		if err != nil {
			return err
		}
		out.GoogleReady = out.StorageReady && apps.Google.Source != "none"
		out.MicrosoftReady = out.StorageReady && apps.Microsoft.Source != "none"
		row, err := store.New(tx).MailboxForDelivery(ctx, p.UserID)
		if database.IsNoRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		provider := api.MailboxProvider(row.Provider)
		address := openapi_types.Email(row.Address)
		port := int(row.Port)
		mode := api.MailboxStatusDeliveryModeLive
		if row.Provider == "smtp" && mail.DeliveryMode(smtpAddress(row.Host, row.Port)) == "test" {
			mode = api.MailboxStatusDeliveryModeTest
		}
		out.DeliveryMode = &mode
		out.Connected = true
		out.Version = int(row.Version)
		out.Provider = &provider
		out.Address = &address
		if row.Provider == "smtp" {
			out.Host = &row.Host
			out.Port = &port
			out.Tls = &row.Tls
			out.Username = &row.Username
		}
		return nil
	})
	return out, err
}
func (s *Service) serverMode() string {
	switch s.connections.ServerMode {
	case "test", "live":
		return s.connections.ServerMode
	default:
		return "unconfigured"
	}
}
func (s *Service) saveSMTP(ctx context.Context, p authz.Principal, in api.SmtpMailboxInput) error {
	port := in.Port
	if port < 1 || port > 65535 {
		return apperr.Validation(apperr.FieldErrors{{Field: "port", Message: "must be between 1 and 65535"}})
	}
	smtpPort := int32(port)
	var errs apperr.FieldErrors
	host := strings.TrimSpace(in.Host)
	if strings.ContainsAny(host, "/:@\\ \t\r\n") || host == "" {
		errs.Add("host", "enter a hostname or public IPv4 address without a port")
	}
	if in.Tls == "none" && !s.connections.AllowPrivate {
		errs.Add("tls", "use STARTTLS or TLS for a live mailbox")
	}
	if len(s.connections.KeysSecret) < 32 {
		errs.Add("password", "mailbox encryption is not configured on this server")
	}
	if err := errs.Err(); err != nil {
		return err
	}
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		old, err := q.MailboxForUser(ctx, p.UserID)
		if err != nil && !database.IsNoRows(err) {
			return err
		}
		version := 0
		if err == nil {
			version = int(old.Version)
		}
		if version != in.ExpectedVersion {
			return apperr.StaleVersion("email connection")
		}
		secret := mailboxSecret{}
		if in.Password == nil || *in.Password == "" {
			// Reusing a secret is safe only for the same destination and identity.
			if old.Provider != "smtp" || old.Host != host || old.Username != in.Username || int(old.Port) != port || old.Address != string(in.Address) || old.Tls != string(in.Tls) {
				if in.Username == "" {
					secret.Password = ""
				} else {
					return apperr.Validation(apperr.FieldErrors{{Field: "password", Message: "enter the password for this connection"}})
				}
			} else {
				if err := s.openMailbox(p.TenantID, p.UserID, "smtp", old.SealedSecret, &secret); err != nil {
					return err
				}
			}
		} else {
			secret.Password = *in.Password
		}
		if in.Username != "" && secret.Password == "" {
			return apperr.Validation(apperr.FieldErrors{{Field: "password", Message: "is required"}})
		}
		sealed, err := s.sealMailbox(p.TenantID, p.UserID, "smtp", secret)
		if err != nil {
			return err
		}
		_, err = q.SaveMailbox(ctx, store.SaveMailboxParams{Provider: "smtp", Address: strings.ToLower(string(in.Address)), Host: host, Port: smtpPort, Tls: string(in.Tls), Username: in.Username, SealedSecret: sealed})
		if err != nil {
			return err
		}
		if err = q.ClearMailboxStates(ctx); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "mailbox.connected", EntityType: "user", EntityID: p.UserID, Changes: map[string]any{"provider": "smtp"}})
	})
}

func (s *Service) disconnectMailbox(ctx context.Context, p authz.Principal) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		if err := q.DeleteMailbox(ctx); err != nil {
			return err
		}
		if err := q.ClearMailboxStates(ctx); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "mailbox.disconnected", EntityType: "user", EntityID: p.UserID})
	})
}

func (s *Service) bindDelivery(ctx context.Context, tx pgx.Tx, emailID, userID string) (string, error) {
	mode := s.serverMode()
	if mode == "unconfigured" {
		mode = "unknown"
	}
	var ref *string
	row, err := store.New(tx).MailboxForDelivery(ctx, userID)
	if err == nil {
		mode = "live"
		if row.Provider == "smtp" && mail.DeliveryMode(smtpAddress(row.Host, row.Port)) == "test" {
			mode = "test"
		}
		ref = &row.ID
	} else if !database.IsNoRows(err) {
		return "", err
	}
	return mode, store.New(tx).BindEmailDelivery(ctx, store.BindEmailDeliveryParams{ID: emailID, DeliveryMode: mode, MailboxRef: ref})
}

// smtpAddress keeps user-supplied hosts out of URI parsing and credentials.
func smtpAddress(host string, port int32) string {
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}
