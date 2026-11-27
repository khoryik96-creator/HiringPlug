package identity

// Which Microsoft account is linked to whom. A first sign-in links the
// account to the team member with its sign-in name (or its email, when
// Microsoft vouches for the email's domain), but never to an owner, nor to
// someone whose account was unlinked: they link one on purpose, from Your
// account while signed in (with their password), or with an invitation or
// reconnect link (microsoft_reconnect.go). Someone made an owner loses a
// link made by an address. Whoever gets an account linked is emailed.
// Owners (and admins, for everyone but owners) unlink it on the Team page;
// that, the company changing its organisation, or turning Microsoft off
// ends the sessions signed in with Microsoft.

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
	linkedBySignIn     = "sign_in"        // its first sign-in, by its address
	linkedByInvitation = "invitation"     // accepting an invitation with it
	linkedBySelf       = "self"           // connected from Your account
	linkedByReconnect  = "reconnect_link" // with a reconnect link an owner sent
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
// organisation), ends any reconnect link of theirs (one is linked now: the
// link has done its work), audits it and emails them. errLinkTaken: the
// account is linked to someone else here; errAlreadyLinked: u has one of
// this organisation already.
func (m *Module) linkMicrosoft(ctx context.Context, tx pgx.Tx, q *store.Queries, u store.UserAuthRow, id entra.Identity, how string) error {
	if err := q.DropStaleMicrosoftLink(ctx, store.DropStaleMicrosoftLinkParams{UserID: u.ID, MsTenantID: id.TenantID}); err != nil {
		return err
	}
	err := q.LinkMicrosoft(ctx, store.LinkMicrosoftParams{UserID: u.ID, MsTenantID: id.TenantID, MsObjectID: id.ObjectID, LinkedBy: how})
	switch {
	case database.IsUniqueViolation(err, "user_microsoft_tenant_id_ms_tenant_id_ms_object_id_key"):
		return errLinkTaken
	case database.IsUniqueViolation(err, "user_microsoft_pkey"):
		return errAlreadyLinked
	case err != nil:
		return err
	}
	if err := q.EndLinks(ctx, store.EndLinksParams{UserID: u.ID, Kind: "reconnect"}); err != nil {
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
	return m.mailer.Notify(ctx, tx, u.TenantID, []string{u.Email}, "Microsoft sign-in connected to your HyPlug account", linkedEmail(u.Name, company.Name, id, how))
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
		linkedByReconnect:  "with the link an owner sent you to connect it",
	}[how]
	return "Hello " + name + ",\n\nThe Microsoft account " + account + " was connected to your HyPlug account at " + company +
		" (" + when + "). It can now sign in to HyPlug as you.\n\n" +
		"Not you? Tell an owner at " + company + " at once: they can unlink it on the Team page.\n\nHyPlug"
}

func notWithAPIKey() error {
	return apperr.New(api.ErrorCodeFORBIDDEN, "Connect Microsoft sign-in while signed in, not with an API key.")
}

// ConnectStart is what starting to connect a Microsoft account takes: the
// browser tab's value (its SHA-256), and proof it is the person signed in:
// their password (if they have one) and two-step code (if they use it).
type ConnectStart struct {
	BindingHash    string
	Password, Code *string
}

// StartConnectingMicrosoft starts connecting the signed-in person's
// Microsoft account (Your account) and returns Microsoft's address. A
// session left open on someone's computer is not enough: it asks for their
// password and two-step code first (limited like changing the password).
func (m *Module) StartConnectingMicrosoft(ctx context.Context, p authz.Principal, clientAddr string, in ConnectStart) (string, error) {
	if !m.limits.msStart.Allow(clientAddr) || !m.limits.change.Allow(p.UserID) {
		return "", tooMany()
	}
	if p.Session().ID == "" {
		return "", notWithAPIKey()
	}
	if m.ms == nil {
		return "", microsoftUnavailable()
	}
	route, err := m.tenantRoute(ctx, p.TenantID)
	if err != nil {
		return "", err
	}
	err = database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		mine, err := q.MyMicrosoft(ctx, p.UserID)
		if err == nil && mine.Linked {
			return connectFailed(ReasonAlreadyConnected)
		}
		if err != nil {
			return err
		}
		u, err := q.UserAuth(ctx, p.UserID)
		if err != nil {
			return err
		}
		return m.proveItIsThem(ctx, q, u, in.Password, in.Code)
	})
	if err != nil {
		return "", err
	}
	route.userID = p.UserID
	return m.startFlow(ctx, route, flowConnect, in.BindingHash, p.Email)
}

// proveItIsThem checks, before a Microsoft account is connected, the
// person's current password (if they have one) and their two-step code or a
// recovery code (if they use two-step sign-in). Codes tried here count
// against the person's limit for codes (as at sign-in): guessing here does
// not add to guessing there.
func (m *Module) proveItIsThem(ctx context.Context, q *store.Queries, u store.UserAuthRow, password, code *string) error {
	var errs apperr.FieldErrors
	if u.PasswordHash != nil && (password == nil || !CheckPassword(*u.PasswordHash, *password)) {
		errs.Add("password", "is not your current password")
	}
	ts, err := q.GetTwoStep(ctx, u.ID)
	if err != nil && !database.IsNoRows(err) {
		return err
	}
	if err == nil && ts.EnabledAt != nil && len(errs) == 0 { // a code is used up only with the right password
		switch {
		case code == nil || *code == "":
			errs.Add("code", "enter the code your authenticator app shows now, or a recovery code")
		case !m.limits.twoStepUser.Allow(u.ID):
			return tooMany()
		default:
			if _, err := m.checkCode(ctx, q, u.TenantID, u.ID, *code, true); err != nil {
				errs.Add("code", "is not the code your app shows now, nor an unused recovery code")
			}
		}
	}
	return errs.Err()
}

// FinishConnectingMicrosoft links the Microsoft account Microsoft vouched
// for to the signed-in person who started connecting it, in the same browser
// tab: an account of the company's organisation, linked to nobody else here.
func (m *Module) FinishConnectingMicrosoft(ctx context.Context, p authz.Principal, clientAddr, ticket, binding string) error {
	if !m.limits.msComplete.Allow(clientAddr) {
		return tooMany()
	}
	if p.Session().ID == "" {
		return notWithAPIKey()
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
		// Held first, like every change of their row and link; then the
		// settings are read: a change of organisation that held them
		// meanwhile (removing the links to the old one) is seen.
		u, err := holdUser(ctx, q, p.UserID)
		if err != nil {
			return err
		}
		s, err := q.MicrosoftSettings(ctx)
		if err != nil {
			return err
		}
		if !s.MicrosoftSignIn || s.MicrosoftTenantID == nil || *s.MicrosoftTenantID != id.TenantID { // changed meanwhile
			return connectFailed(ReasonFailed)
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
// their sessions signed in with it (password sessions go on), and any
// reconnect link sent them. From then on one is linked to them only on
// purpose (Your account, a new reconnect link), never by a sign-in's
// address: it was unlinked for a reason. Only owners unlink owners, and
// nobody someone whose role is above their own. Their row is held first
// (as a sign-in holds it before their link): the two wait for each other,
// never deadlock.
func (m *Module) UnlinkMicrosoft(ctx context.Context, p authz.Principal, userID string) error {
	return database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		u, err := holdUser(ctx, q, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("team member")
		}
		if err != nil {
			return err
		}
		switch {
		case u.Role == string(api.RoleOwner) && p.Role != api.RoleOwner:
			return teamProblem("Only an owner can unlink an owner's Microsoft account.")
		case RoleRank(u.Role) > RoleRank(string(p.Role)):
			return teamProblem("Only an admin or owner can unlink an admin's Microsoft account.")
		}
		org, err := q.UnlinkMicrosoft(ctx, userID)
		if database.IsNoRows(err) {
			return apperr.NotFound("linked Microsoft account")
		}
		if err != nil {
			return err
		}
		if err := q.MicrosoftConnectOnly(ctx, userID); err != nil {
			return err
		}
		if err := q.EndMicrosoftSessions(ctx, &userID); err != nil {
			return err
		}
		if err := q.EndLinks(ctx, store.EndLinksParams{UserID: userID, Kind: "reconnect"}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "user.microsoft_unlinked", EntityType: "user", EntityID: userID,
			Changes: map[string]any{"organisationId": org}})
	})
}

// MicrosoftSignInOn reports whether the caller's company signs in with
// Microsoft, in the caller's transaction (the emails module keeps its
// Microsoft app for owners then), and keeps it so until that transaction
// ends: turning it on (SetMicrosoftSignIn) waits for a change of the app.
func (m *Module) MicrosoftSignInOn(ctx context.Context, tx pgx.Tx) (bool, error) {
	return store.New(tx).MicrosoftSignInHeld(ctx)
}

// dropAddressLinkOfNewOwner removes, from someone made an owner, a Microsoft
// account linked by a sign-in's address (owners link one only on purpose),
// and ends their Microsoft sessions; audited. From then on one is linked to
// them only on purpose, even once they are no longer an owner.
func (m *Module) dropAddressLinkOfNewOwner(ctx context.Context, tx pgx.Tx, q *store.Queries, userID string) error {
	if err := q.MicrosoftConnectOnly(ctx, userID); err != nil {
		return err
	}
	orgs, err := q.UnlinkSignInLink(ctx, userID)
	if err != nil || len(orgs) == 0 {
		return err
	}
	if err := q.EndMicrosoftSessions(ctx, &userID); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "user.microsoft_unlinked", EntityType: "user", EntityID: userID,
		Changes: map[string]any{"organisationId": orgs[0], "reason": "made_owner"}})
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
