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
