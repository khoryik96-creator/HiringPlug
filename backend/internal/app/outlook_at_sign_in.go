package app

import (
	"context"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/calendars"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/emails"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// outlookAtSignIn connects a person's Outlook mailbox (emails module) and
// calendar (calendars module) with the Outlook part of their Microsoft
// sign-in. Each goes its own way: what one could not do, the person still
// does in Settings, and the sign-in itself is never failed by either.
type outlookAtSignIn struct {
	emails    *emails.Module
	calendars *calendars.Module
}

func (o outlookAtSignIn) ConnectOutlook(ctx context.Context, p authz.Principal, g identity.OutlookGrant) {
	mail, err := o.emails.ConnectFromSignIn(ctx, p, emails.SignInGrant{
		ClientID: g.ClientID, AccessToken: g.AccessToken, RefreshToken: g.RefreshToken, ExpiresIn: g.ExpiresIn, Scope: g.Scope})
	if err != nil {
		logx.From(ctx).Warn("microsoft sign-in: Outlook mail not connected", "error", err)
	}
	cal, err := o.calendars.ConnectFromSignIn(ctx, p, calendars.SignInGrant{
		ClientID: g.ClientID, AccessToken: g.AccessToken, RefreshToken: g.RefreshToken, ExpiresIn: g.ExpiresIn, Scope: g.Scope})
	if err != nil {
		logx.From(ctx).Warn("microsoft sign-in: Outlook calendar not connected", "error", err)
	}
	logx.From(ctx).Info("microsoft sign-in: Outlook", "mail", mail, "calendar", cal)
}
