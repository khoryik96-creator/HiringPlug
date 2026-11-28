package emails

// Connecting the Outlook mailbox when someone signs in with Microsoft (the
// company connects Outlook at sign-in): the sign-in's Graph tokens, of the
// company's Microsoft app, connect the mailbox as Settings > Email connection
// would, once Microsoft has said whose it is. A mailbox they connected to
// another account (or another provider, or SMTP) is theirs to change: it is
// left as it is.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// SignInGrant is the Outlook part of a Microsoft sign-in: Graph tokens of
// the app it went through.
type SignInGrant struct {
	ClientID                  string
	AccessToken, RefreshToken string
	ExpiresIn                 int    // seconds
	Scope                     string // what Microsoft granted
}

// Granted reports whether the grant includes every one of scopes (Graph
// scopes, with or without their https://graph.microsoft.com/ prefix).
func (g SignInGrant) Granted(scopes ...string) bool {
	have := map[string]bool{}
	for _, s := range strings.Fields(g.Scope) {
		have[strings.ToLower(strings.TrimPrefix(s, "https://graph.microsoft.com/"))] = true
	}
	for _, s := range scopes {
		if !have[strings.ToLower(s)] {
			return false
		}
	}
	return true
}

// What connecting at sign-in did.
const (
	SignInConnected = "connected" // a mailbox connected where there was none
	SignInRefreshed = "refreshed" // the same Outlook mailbox, its tokens renewed
	SignInLeft      = "left"      // another mailbox stays connected; nothing changed
	SignInSkipped   = "skipped"   // another app, or mail not granted: nothing changed
)

// ConnectFromSignIn connects (or refreshes) the person's Outlook mailbox with
// the Outlook part of their Microsoft sign-in, and says what it did.
func (m *Module) ConnectFromSignIn(ctx context.Context, p authz.Principal, g SignInGrant) (string, error) {
	s := m.service
	app, err := s.appIn(ctx, p, ProviderMicrosoft)
	if err != nil {
		return "", err
	}
	if app.ClientID == "" || app.ClientID != g.ClientID || g.RefreshToken == "" || !g.Granted("Mail.Send", "Mail.ReadWrite") {
		return SignInSkipped, nil // its tokens would not refresh through the company's app
	}
	if g.ExpiresIn <= 0 || g.ExpiresIn > 86400 {
		return "", errors.New("emails: the sign-in's mail authorization is not valid")
	}
	profile, err := s.mailboxProfile(ctx, ProviderMicrosoft, s.endpoint(ProviderMicrosoft), g.AccessToken)
	if err != nil {
		return "", err
	}
	sealed, err := s.sealMailbox(p.TenantID, p.UserID, ProviderMicrosoft, mailboxSecret{
		AccountID: profile.accountID, AccessToken: g.AccessToken, RefreshToken: g.RefreshToken,
		ExpiresAt: time.Now().Add(time.Duration(g.ExpiresIn) * time.Second),
	})
	if err != nil {
		return "", err
	}
	var did string
	err = database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockMailboxUser(ctx); err != nil {
			return err
		}
		box, err := q.MailboxForUser(ctx, p.UserID)
		switch {
		case database.IsNoRows(err):
			if _, err := q.SaveMailbox(ctx, store.SaveMailboxParams{Provider: ProviderMicrosoft, Address: profile.address, SealedSecret: sealed}); err != nil {
				return err
			}
			did = SignInConnected
			return audit.Record(ctx, tx, audit.Entry{Action: "mailbox.connected", EntityType: "user", EntityID: p.UserID,
				Changes: map[string]any{"provider": ProviderMicrosoft, "via": "microsoft_sign_in"}})
		case err != nil:
			return err
		case s.sameOutlookMailbox(box, ProviderMicrosoft, profile):
			if err := q.RefreshMailboxSecret(ctx, store.RefreshMailboxSecretParams{ID: box.ID, SealedSecret: sealed}); err != nil {
				return err
			}
			did = SignInRefreshed
			return audit.Record(ctx, tx, audit.Entry{Action: "mailbox.reconnected", EntityType: "user", EntityID: p.UserID,
				Changes: map[string]any{"provider": ProviderMicrosoft, "via": "microsoft_sign_in"}})
		}
		did = SignInLeft
		return nil
	})
	return did, err
}
