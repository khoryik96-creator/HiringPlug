package identity

// Linking a Microsoft account through a link: an invitation, or a reconnect
// link an owner sends from the Team page (someone whose account was
// unlinked, or whose sign-in name is not their HiringPlug email). Signing in
// with Microsoft from the link links the account to that person, whatever
// its address: an account of the company's organisation, linked to nobody
// else there. Like an invitation, a reconnect link works once, for 7 days,
// and is emailed to the person's HiringPlug address.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
)

// signInThroughLink signs in the person whose invitation or reconnect link
// started the sign-in, linking the Microsoft account to them (or using the
// one linked already), with that very link: held, it must still be unused,
// unexpired, of the same kind and theirs, and it is used up here. A sign-in
// started from an invitation they accepted with a password since, or from
// a link replaced by a newer one, links nothing (even if another link of
// theirs works now); two finishing with one link at once: one works.
func (m *Module) signInThroughLink(ctx context.Context, flow store.TakeMicrosoftFlowRow, id entra.Identity) (Session, error) {
	var out Session
	err := database.InTx(ctx, m.pool, authz.NewPrincipal(flow.TenantID, flow.UserID, "", "", ""), func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := m.microsoftUser(ctx, q, flow.UserID, id)
		if err != nil {
			return err
		}
		l, err := q.HoldLink(ctx, flow.LinkID)
		if database.IsNoRows(err) || (err == nil && (!l.Valid || l.Kind != flow.LinkKind || l.UserID != u.ID || (l.Kind == "invite") != u.Invited)) {
			return microsoftFailed(ReasonFailed) // used, replaced or expired meanwhile
		}
		if err != nil {
			return err
		}
		how := map[string]string{"invite": linkedByInvitation, "reconnect": linkedByReconnect}[l.Kind]
		holder, err := q.MicrosoftAccountHolder(ctx, store.MicrosoftAccountHolderParams{MsTenantID: id.TenantID, MsObjectID: id.ObjectID})
		switch {
		case err == nil && holder == u.ID: // linked to them already
			_, err = q.UsedMicrosoft(ctx, store.UsedMicrosoftParams{UserID: u.ID, MsTenantID: id.TenantID, MsObjectID: id.ObjectID})
		case err == nil:
			return microsoftFailed(ReasonLinkedElsewhere)
		case database.IsNoRows(err):
			err = m.linkMicrosoft(ctx, tx, q, u, id, how)
			switch {
			case errors.Is(err, errLinkTaken):
				return microsoftFailed(ReasonLinkedElsewhere)
			case errors.Is(err, errAlreadyLinked):
				return microsoftFailed(ReasonAlreadyConnected)
			}
		}
		if err != nil {
			return err
		}
		if err := q.EndLinks(ctx, store.EndLinksParams{UserID: u.ID, Kind: l.Kind}); err != nil {
			return err
		}
		out, err = m.signedInWithMicrosoft(ctx, tx, q, u)
		return err
	})
	return out, err
}

// ReconnectLink is a link sent for someone to connect their Microsoft
// account: when it expires (the link itself goes only to their email).
type ReconnectLink struct {
	ExpiresAt time.Time
	Emailed   bool
}

// SendMicrosoftReconnectLink emails someone on the team a link to connect
// their Microsoft account, whatever its address (replacing an older one).
// Only to their email: whoever held the link could link an account of
// theirs to that person, and sign in as them. Only owners send one to an
// owner, and nobody to someone whose role is above their own (a teammate let
// to manage the team, to an admin); nobody to themselves (they connect from
// Your account, with their password). Their row is held first, so a
// deactivation or a change of role under way finishes before they are
// checked (and ends the links sent before it).
func (m *Module) SendMicrosoftReconnectLink(ctx context.Context, p authz.Principal, userID string) (ReconnectLink, error) {
	var out ReconnectLink
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := holdUser(ctx, q, userID) // as every change of their link: a change of them under way finishes first
		if database.IsNoRows(err) {
			return apperr.NotFound("team member")
		}
		if err != nil {
			return err
		}
		switch {
		case u.ID == p.UserID:
			return teamProblem("Connect your own Microsoft account from Your account.")
		case u.Role == string(api.RoleOwner) && p.Role != api.RoleOwner:
			return teamProblem("Only an owner can send an owner a link to connect Microsoft.")
		case RoleRank(u.Role) > RoleRank(string(p.Role)): // whoever holds it signs in as them
			return teamProblem("Only an admin or owner can send an admin a link to connect Microsoft.")
		case u.Invited:
			return teamProblem("They have not joined yet: send a new invitation instead (they can accept it with Microsoft).")
		case !u.Active:
			return teamProblem("They are deactivated: reactivate them first.")
		case !u.MicrosoftSignIn || m.ms == nil:
			return teamProblem("Your company does not sign in with Microsoft: turn it on in Roles and access first.")
		}
		link, expires, err := m.link(ctx, q, u.ID, "reconnect", InviteValid)
		if err != nil {
			return err
		}
		company, err := q.GetTenant(ctx, p.TenantID)
		if err != nil {
			return err
		}
		body := "Hello " + u.Name + ",\n\n" + p.Name + " sent you a link to connect your Microsoft work account to HiringPlug at " + company.Name +
			", so you can sign in with it.\n\nOpen it and sign in with Microsoft (the link works once, for 7 days):\n" + link +
			"\n\nIf you did not expect this, ignore it and tell " + p.Name + ".\n\nHiringPlug"
		if err := m.mailer.Notify(ctx, tx, p.TenantID, []string{u.Email}, "Connect Microsoft sign-in to HiringPlug at "+company.Name, body); err != nil {
			return err
		}
		out = ReconnectLink{ExpiresAt: expires, Emailed: true}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.microsoft_reconnect_sent", EntityType: "user", EntityID: u.ID})
	})
	return out, err
}
