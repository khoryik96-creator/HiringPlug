package emails

import (
	"context"

	"github.com/jackc/pgx/v5"
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
