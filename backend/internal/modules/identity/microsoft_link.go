package identity

// Which Microsoft account is linked to whom. A first sign-in links the
// account to the team member with its sign-in name (or its email, when
// Microsoft vouches for the email's domain), but never to an owner: owners
// connect it from Your account while signed in (or accept their invitation
// with it). Whoever gets an account linked is emailed. Owners (and admins,
// for everyone but owners) unlink it on the Team page; that, the company
// changing its organisation, or turning Microsoft off ends the sessions
// signed in with Microsoft.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// How a Microsoft account came to be linked (user.microsoft_linked's by).
const (
	linkedBySignIn     = "sign_in"    // its first sign-in, by its address
	linkedByInvitation = "invitation" // accepting an invitation with it
	linkedBySelf       = "self"       // connected from Your account
)

var (
	errLinkTaken     = errors.New("the Microsoft account is linked to someone else")
	errAlreadyLinked = errors.New("a Microsoft account is linked to them already")
)

var connectMessages = map[string]string{
	ReasonAlreadyConnected:  "A Microsoft account is already connected to you. To use another, ask an owner to unlink it on the Team page first.",
	ReasonLinkedElsewhere:   "This Microsoft account is already connected to someone else on the team.",
	ReasonOtherOrganisation: "This Microsoft account belongs to another organisation (or is a guest in yours). Use your work account at this company.",
	ReasonPersonalAccount:   "This is a personal Microsoft account. Use your work account.",
	ReasonFailed:            "Connecting your Microsoft account didn't work or has expired. Start again.",
}

func connectFailed(reason string) error {
	return apperr.New(api.ErrorCodeMICROSOFTCONNECTFAILED, connectMessages[reason]).WithDetail("reason", reason)
}

// asConnectFailed says a sign-in refusal (microsoftFailed) as a connect one.
func asConnectFailed(err error) error {
	if e, ok := apperr.As(err); ok && e.Code == api.ErrorCodeMICROSOFTSIGNINFAILED {
		reason, _ := e.Details["reason"].(string)
		if _, known := connectMessages[reason]; !known {
			reason = ReasonFailed
		}
		return connectFailed(reason)
	}
	return err
}

// linkMicrosoft links a Microsoft account to u (replacing a link to another
// organisation), audits it and emails them. errLinkTaken: the account is
// linked to someone else here; errAlreadyLinked: u has one of this
// organisation already.
func (m *Module) linkMicrosoft(ctx context.Context, tx pgx.Tx, q *store.Queries, u store.UserAuthRow, id entra.Identity, how string) error {
	if err := q.DropStaleMicrosoftLink(ctx, store.DropStaleMicrosoftLinkParams{UserID: u.ID, MsTenantID: id.TenantID}); err != nil {
		return err
	}
	err := q.LinkMicrosoft(ctx, store.LinkMicrosoftParams{UserID: u.ID, MsTenantID: id.TenantID, MsObjectID: id.ObjectID})
	switch {
	case database.IsUniqueViolation(err, "user_microsoft_tenant_id_ms_tenant_id_ms_object_id_key"):
		return errLinkTaken
	case database.IsUniqueViolation(err, "user_microsoft_pkey"):
		return errAlreadyLinked
	case err != nil:
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: "user.microsoft_linked", EntityType: "user", EntityID: u.ID,
		Changes: map[string]any{"organisationId": id.TenantID, "by": how}}); err != nil {
		return err
	}
	company, err := q.GetTenant(ctx, u.TenantID)
	if err != nil {
		return err
	}
	return m.mailer.Notify(ctx, tx, u.TenantID, []string{u.Email}, "Microsoft sign-in connected to your HiringPlug account", linkedEmail(u.Name, company.Name, id, how))
}

// linkedEmail tells someone a Microsoft account was linked to them.
func linkedEmail(name, company string, id entra.Identity, how string) string {
	account := id.Username
	if account == "" {
		account = id.Email
	}
	when := map[string]string{
		linkedBySignIn:     "when it was first used to sign in",
		linkedByInvitation: "when your invitation was accepted with it",
		linkedBySelf:       "from Your account",
	}[how]
	return "Hello " + name + ",\n\nThe Microsoft account " + account + " was connected to your HiringPlug account at " + company +
		" (" + when + "). It can now sign in to HiringPlug as you.\n\n" +
		"Not you? Tell an owner at " + company + " at once: they can unlink it on the Team page.\n\nHiringPlug"
}

// StartConnectingMicrosoft starts connecting the signed-in person's
// Microsoft account (Your account) and returns Microsoft's address.
func (m *Module) StartConnectingMicrosoft(ctx context.Context, p authz.Principal, clientAddr, bindingHash string) (string, error) {
	if !m.limits.msStart.Allow(clientAddr) {
		return "", tooMany()
	}
	if p.Session().ID == "" {
		return "", apperr.New(api.ErrorCodeFORBIDDEN, "Connect Microsoft sign-in while signed in, not with an API key.")
	}
	if m.ms == nil {
		return "", microsoftUnavailable()
	}
	route, err := m.tenantRoute(ctx, p.TenantID)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		mine, err := store.New(tx).MyMicrosoft(ctx, p.UserID)
		if err == nil && mine.Linked {
			return connectFailed(ReasonAlreadyConnected)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	route.userID = p.UserID
	return m.startFlow(ctx, route, flowConnect, bindingHash, p.Email)
}

// FinishConnectingMicrosoft links the Microsoft account Microsoft vouched
// for to the signed-in person who started connecting it, in the same browser
// tab: an account of the company's organisation, linked to nobody else here.
func (m *Module) FinishConnectingMicrosoft(ctx context.Context, p authz.Principal, clientAddr, ticket, binding string) error {
	if !m.limits.msComplete.Allow(clientAddr) {
		return tooMany()
	}
	if m.ms == nil {
		return connectFailed(ReasonFailed)
	}
	flow, err := store.New(m.pool).TakeMicrosoftFlow(ctx, hashLink(ticket))
	if database.IsNoRows(err) {
		return connectFailed(ReasonFailed)
	}
	if err != nil {
		return err
	}
	if !BindingMatches(flow.BindingHash, binding) || flow.Purpose != flowConnect || flow.UserID != p.UserID || flow.TenantID != p.TenantID {
		logx.From(ctx).Info("microsoft connect refused", "reason", "another browser, person or purpose")
		return connectFailed(ReasonFailed)
	}
	id, err := m.microsoftIdentity(ctx, flow)
	if err != nil {
		return asConnectFailed(err)
	}
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		s, err := q.MicrosoftSettings(ctx)
		if err != nil {
			return err
		}
		if !s.MicrosoftSignIn || s.MicrosoftTenantID == nil || *s.MicrosoftTenantID != id.TenantID { // changed meanwhile
			return connectFailed(ReasonFailed)
		}
		u, err := q.UserAuth(ctx, p.UserID)
		if err != nil {
			return err
		}
		switch err := m.linkMicrosoft(ctx, tx, q, u, id, linkedBySelf); {
		case errors.Is(err, errLinkTaken):
			return connectFailed(ReasonLinkedElsewhere)
		case errors.Is(err, errAlreadyLinked):
			return connectFailed(ReasonAlreadyConnected)
		default:
			return err
		}
	})
}

// UnlinkMicrosoft removes the Microsoft account linked to someone and ends
// their sessions signed in with it (password sessions go on). Only owners
// unlink owners.
func (m *Module) UnlinkMicrosoft(ctx context.Context, p authz.Principal, userID string) error {
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := q.UserAuth(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("team member")
		}
		if err != nil {
			return err
		}
		if u.Role == string(api.RoleOwner) && p.Role != api.RoleOwner {
			return teamProblem("Only an owner can unlink an owner's Microsoft account.")
		}
		org, err := q.UnlinkMicrosoft(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("linked Microsoft account")
		}
		if err != nil {
			return err
		}
		if err := q.EndMicrosoftSessions(ctx, store.EndMicrosoftSessionsParams{At: sessionsEndedAt(), UserID: &userID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.microsoft_unlinked", EntityType: "user", EntityID: userID,
			Changes: map[string]any{"organisationId": org}})
	})
}

// MicrosoftSignInOn reports whether the caller's company signs in with
// Microsoft, in the caller's transaction (the emails module keeps its
// Microsoft app for owners then).
func (m *Module) MicrosoftSignInOn(ctx context.Context, tx pgx.Tx) (bool, error) {
	s, err := store.New(tx).MicrosoftSettings(ctx)
	return s.MicrosoftSignIn, err
}

// MyMicrosoft says whether the person's company signs in with Microsoft
// (on this server), and whether a Microsoft account is linked to them.
func (m *Module) MyMicrosoft(ctx context.Context, p authz.Principal) (signIn, linked bool, err error) {
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).MyMicrosoft(ctx, p.UserID)
		signIn, linked = row.SignIn && m.ms != nil, row.Linked
		return err
	})
	return signIn, linked, err
}
