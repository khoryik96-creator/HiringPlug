package identity

// Connecting Outlook at sign-in: a company that signs in with Microsoft may
// ask, in the same Microsoft consent, for each person's Outlook mail and
// calendar too (MicrosoftSignIn.ConnectsOutlook, entra.OutlookScope). Once
// they are signed in, the grant goes to whoever keeps mailboxes and calendars
// (the emails and calendars modules, wired in internal/app): they connect
// it, or refresh a connection of the same account, and leave one of another
// account alone. Nothing about it ever fails the sign-in.

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// OutlookGrant is what a Microsoft sign-in that asked for Outlook gives:
// Graph tokens of the app the sign-in went through.
type OutlookGrant struct {
	ClientID                  string
	AccessToken, RefreshToken string
	ExpiresIn                 int    // seconds
	Scope                     string // what was granted
}

// OutlookConnector connects a signed-in person's Outlook mail and calendar
// with a grant (and says nothing: what it could not connect, the person
// connects as before, in Settings).
type OutlookConnector interface {
	ConnectOutlook(ctx context.Context, p authz.Principal, g OutlookGrant)
}

// ConnectOutlookWith gives the module what connects Outlook at sign-in.
func (m *Module) ConnectOutlookWith(c OutlookConnector) { m.outlook = c }

// outlookGrant is the grant of a sign-in that asked for Outlook, when
// Microsoft gave one (nil otherwise: the person may have declined part of it).
func outlookGrant(asked bool, clientID string, t entra.Tokens) *OutlookGrant {
	if !asked || t.AccessToken == "" || t.RefreshToken == "" {
		return nil
	}
	return &OutlookGrant{ClientID: clientID, AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, ExpiresIn: t.ExpiresIn, Scope: t.Scope}
}

// connectOutlook hands a sign-in's grant over, as the person just signed in.
func (m *Module) connectOutlook(ctx context.Context, sess Session, g *OutlookGrant) {
	if g == nil || m.outlook == nil {
		return
	}
	p, err := m.Authenticate(ctx, sess.Token)
	if err != nil {
		logx.From(ctx).Warn("microsoft sign-in: Outlook not connected", "error", err)
		return
	}
	m.outlook.ConnectOutlook(ctx, p, *g)
}
