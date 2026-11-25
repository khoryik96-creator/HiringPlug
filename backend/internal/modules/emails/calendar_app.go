package emails

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// MicrosoftCalendarApplication lets app wiring reuse the agency's Entra app
// registration. It exposes no mailbox tokens and adds no calendar mail scope.
// This internal boundary is never returned through an HTTP response.
func (m *Module) MicrosoftCalendarApplication(ctx context.Context, tx pgx.Tx, tenant string) (OAuthApplication, error) {
	return m.service.appFor(ctx, tx, tenant, "microsoft")
}

// StorageApplication lets app wiring reuse the agency's Microsoft or Google
// app registration (provider "microsoft" or "google") for the claims
// storage (OneDrive, Google Drive). It exposes no mailbox tokens.
func (m *Module) StorageApplication(ctx context.Context, tx pgx.Tx, tenant, provider string) (OAuthApplication, error) {
	if provider != "microsoft" && provider != "google" {
		return OAuthApplication{}, nil
	}
	return m.service.appFor(ctx, tx, tenant, provider)
}

// MicrosoftSignInApplication lets app wiring reuse the agency's Microsoft app
// (or else the server's) for signing in with Microsoft; agency says whose it
// is. It exposes no mailbox tokens.
func (m *Module) MicrosoftSignInApplication(ctx context.Context, tx pgx.Tx, tenant string) (app OAuthApplication, agency bool, err error) {
	_, err = store.New(tx).MailApp(ctx, ProviderMicrosoft)
	if err != nil && !database.IsNoRows(err) {
		return OAuthApplication{}, false, err
	}
	agency = err == nil
	app, err = m.service.appFor(ctx, tx, tenant, ProviderMicrosoft)
	return app, agency, err
}

// ServerMicrosoftApplication is the server's own Microsoft app
// (HP_MAIL_MICROSOFT_*), for signing in before a company is known.
func (m *Module) ServerMicrosoftApplication() OAuthApplication {
	return m.service.serverApp(ProviderMicrosoft)
}
