package app

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
)

// identityMicrosoftApps lends identity the Microsoft app the emails module
// keeps (the agency's, or the server's HP_MAIL_MICROSOFT_*), to sign people
// in with Microsoft. Only the app registration crosses, never a mailbox.
type identityMicrosoftApps struct{ emails *emails.Module }

func (a identityMicrosoftApps) MicrosoftSignInApp(ctx context.Context, tx pgx.Tx, tenantID string) (identity.MicrosoftApp, error) {
	app, agency, err := a.emails.MicrosoftSignInApplication(ctx, tx, tenantID)
	return identity.MicrosoftApp{ClientID: app.ClientID, ClientSecret: app.ClientSecret, Agency: agency}, err
}

func (a identityMicrosoftApps) ServerMicrosoftApp() identity.MicrosoftApp {
	app := a.emails.ServerMicrosoftApplication()
	return identity.MicrosoftApp{ClientID: app.ClientID, ClientSecret: app.ClientSecret}
}
