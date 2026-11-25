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
// one linked already), while the link is still unused and unexpired.
func (m *Module) signInThroughLink(ctx context.Context, flow store.TakeMicrosoftFlowRow, id entra.Identity) (Session, error) {
	var out Session
	err := database.InTx(ctx, m.pool, authz.NewPrincipal(flow.TenantID, flow.UserID, "", "", ""), func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := m.microsoftUser(ctx, q, flow.UserID, id)
		if err != nil {
			return err
		}
		kind, how := "reconnect", linkedByReconnect
		if u.Invited {
			kind, how = "invite", linkedByInvitation
		}
		valid, err := q.HasValidLink(ctx, store.HasValidLinkParams{UserID: u.ID, Kind: kind})
		if err != nil {
			return err
		}
		if !valid { // used, replaced or expired meanwhile
			return microsoftFailed(ReasonFailed)
		}
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
		if err := q.EndLinks(ctx, store.EndLinksParams{UserID: u.ID, Kind: kind}); err != nil {
			return err
		}
		out, err = m.signedInWithMicrosoft(ctx, tx, q, u)
		return err
	})
	return out, err
}

// ReconnectLink is a link sent for someone to connect their Microsoft account.
type ReconnectLink struct {
	Link      string
	ExpiresAt time.Time
	Emailed   bool
}

// SendMicrosoftReconnectLink emails someone on the team a link to connect
// their Microsoft account, whatever its address (replacing an older one).
// Only owners send one to an owner.
func (m *Module) SendMicrosoftReconnectLink(ctx context.Context, p authz.Principal, userID string) (ReconnectLink, error) {
	var out ReconnectLink
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("team member")
		}
		if err != nil {
			return err
		}
		switch {
		case u.Role == string(api.RoleOwner) && p.Role != api.RoleOwner:
			return teamProblem("Only an owner can send an owner a link to connect Microsoft.")
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
		out = ReconnectLink{Link: link, ExpiresAt: expires, Emailed: true}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.microsoft_reconnect_sent", EntityType: "user", EntityID: u.ID})
	})
	return out, err
}
