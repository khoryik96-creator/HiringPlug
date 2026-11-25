package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

var microsoftMessages = map[string]string{
	ReasonNotOnTeam:         "Your Microsoft account isn't on the HiringPlug team. Ask an owner to invite you.",
	ReasonOtherOrganisation: "This Microsoft account belongs to another organisation. Sign in with your work account at this company.",
	ReasonPersonalAccount:   "This is a personal Microsoft account. Sign in with your work account.",
	ReasonConnectFirst:      "Owners connect their Microsoft account once before signing in with it: sign in with your password, then use Your account › Connect Microsoft sign-in.",
	ReasonFailed:            "Signing in with Microsoft didn't work or has expired. Start again.",
}

func microsoftFailed(reason string) error {
	return apperr.New(api.ErrorCodeMICROSOFTSIGNINFAILED, microsoftMessages[reason]).WithDetail("reason", reason)
}

// MicrosoftCompletion finishes a sign-in: the ticket, the browser tab's
// value, and the company chosen (when the account is at several).
type MicrosoftCompletion struct{ Ticket, Binding, Company string }

// CompleteMicrosoft finishes a sign-in in the browser tab that started it:
// the ticket works once, with the tab's value only. The code is exchanged,
// the ID token checked, and the account matched to a team member.
func (m *Module) CompleteMicrosoft(ctx context.Context, clientAddr string, in MicrosoftCompletion) (Session, error) {
	if !m.limits.msComplete.Allow(clientAddr) {
		return Session{}, tooMany()
	}
	if m.ms == nil {
		return Session{}, microsoftFailed(ReasonFailed)
	}
	flow, err := store.New(m.pool).TakeMicrosoftFlow(ctx, hashLink(in.Ticket))
	if database.IsNoRows(err) {
		return Session{}, microsoftFailed(ReasonFailed)
	}
	if err != nil {
		return Session{}, err
	}
	if !BindingMatches(flow.BindingHash, in.Binding) || flow.Purpose != flowSignIn { // the ticket is gone now too
		logx.From(ctx).Info("microsoft sign-in refused", "reason", "finished in another browser, or not a sign-in")
		return Session{}, microsoftFailed(ReasonFailed)
	}
	id, err := m.microsoftIdentity(ctx, flow)
	if err != nil {
		return Session{}, err
	}
	rows, err := store.New(m.pool).MicrosoftAccounts(ctx, store.MicrosoftAccountsParams{MsTenantID: id.TenantID, MsObjectID: id.ObjectID, Addresses: id.Addresses()})
	if err != nil {
		return Session{}, err
	}
	found := make([]MicrosoftAccount, len(rows))
	for i, r := range rows {
		found[i] = MicrosoftAccount{TenantID: r.TenantID, TenantName: r.TenantName, Slug: r.CareersSlug, UserID: r.UserID, Linked: r.Linked, Owner: r.IsOwner}
	}
	accounts, ownersLeftOut := ChooseMicrosoftAccounts(found, flow.TenantID, in.Company, flow.UserID)
	switch len(accounts) {
	case 0:
		if ownersLeftOut {
			return Session{}, microsoftFailed(ReasonConnectFirst)
		}
		return Session{}, microsoftFailed(ReasonNotOnTeam)
	case 1:
		return m.signInWithMicrosoft(ctx, accounts[0], id)
	}
	return Session{}, m.chooseCompany(ctx, flow, id, accounts)
}

// microsoftIdentity is the Microsoft account a sign-in is for: checked now
// (the code exchanged, its ID token validated), or before a company choice.
func (m *Module) microsoftIdentity(ctx context.Context, flow store.TakeMicrosoftFlowRow) (entra.Identity, error) {
	var id entra.Identity
	if flow.Verified {
		if err := m.ms.openJSON("identity:"+flow.KeyHash, flow.Sealed, &id); err != nil {
			return id, microsoftFailed(ReasonFailed)
		}
		return id, nil
	}
	var secret flowSecret
	var code string
	if flow.SealedCode == "" || m.ms.openJSON("flow:"+flow.KeyHash, flow.Sealed, &secret) != nil ||
		m.ms.openJSON("code:"+flow.KeyHash, flow.SealedCode, &code) != nil {
		return id, microsoftFailed(ReasonFailed)
	}
	app, organisation, err := m.flowApp(ctx, flow)
	if err != nil {
		return id, err
	}
	raw, err := m.ms.entra.IDToken(ctx, entra.Exchange{Authority: flow.Authority, ClientID: app.ClientID, ClientSecret: app.ClientSecret,
		Code: code, RedirectURI: m.microsoftCallback(), Verifier: secret.Verifier})
	if err == nil {
		id, err = m.ms.entra.Validate(ctx, raw, entra.Expect{ClientID: flow.ClientID, Nonce: secret.Nonce, Organisation: organisation})
	}
	if err != nil {
		logx.From(ctx).Info("microsoft sign-in refused", "reason", err.Error())
		switch {
		case errors.Is(err, entra.ErrPersonalAccount):
			return id, microsoftFailed(ReasonPersonalAccount)
		case errors.Is(err, entra.ErrOtherOrganisation), errors.Is(err, entra.ErrGuestAccount): // a guest's home is another organisation
			return id, microsoftFailed(ReasonOtherOrganisation)
		}
		return id, microsoftFailed(ReasonFailed)
	}
	return id, nil
}

// flowApp is the app a sign-in was started with, as it is now, and the
// organisation its company accepts ("" for any, through the server's app).
// A company that stopped signing in with Microsoft, or changed its app,
// meanwhile fails the sign-in.
func (m *Module) flowApp(ctx context.Context, flow store.TakeMicrosoftFlowRow) (MicrosoftApp, string, error) {
	app, organisation := m.ms.apps.ServerMicrosoftApp(), ""
	if flow.TenantID != "" {
		err := database.InTx(ctx, m.pool, authz.System(flow.TenantID), func(tx pgx.Tx) error {
			var err error
			organisation, app, err = m.tenantMicrosoft(ctx, tx, store.New(tx), flow.TenantID)
			return err
		})
		if _, unavailable := apperr.As(err); unavailable {
			return app, "", microsoftFailed(ReasonFailed)
		}
		if err != nil {
			return app, "", err
		}
	}
	if !app.usable() || app.ClientID != flow.ClientID {
		return app, "", microsoftFailed(ReasonFailed)
	}
	return app, organisation, nil
}

// chooseCompany answers an account at several companies with them, and a
// new one-time ticket (for the same browser tab) to finish with one.
func (m *Module) chooseCompany(ctx context.Context, flow store.TakeMicrosoftFlowRow, id entra.Identity, accounts []MicrosoftAccount) error {
	key, ticket := hashLink(randomToken()), randomToken()
	sealed, err := m.ms.sealJSON("identity:"+key, id)
	if err != nil {
		return err
	}
	ticketHash := hashLink(ticket)
	err = store.New(m.pool).SaveMicrosoftFlow(ctx, store.SaveMicrosoftFlowParams{
		KeyHash: key, BindingHash: flow.BindingHash, Purpose: flowSignIn, UserID: orNil(flow.UserID), ClientID: flow.ClientID, Authority: flow.Authority,
		Sealed: sealed, Verified: true, TicketHash: &ticketHash, ExpiresAt: time.Now().Add(microsoftChoiceLife),
	})
	if err != nil {
		return err
	}
	companies := make([]map[string]string, len(accounts))
	for i, a := range accounts {
		companies[i] = map[string]string{"slug": a.Slug, "name": a.TenantName}
	}
	return apperr.New(api.ErrorCodeSIGNINCHOOSECOMPANY, "Your Microsoft account is at several companies: choose one.").
		WithDetail("companies", companies).WithDetail("ticket", ticket)
}

// signInWithMicrosoft signs a team member in: their Microsoft account is
// linked the first time, an invitation is accepted, and the session says it
// was signed in with Microsoft (no two-step code).
func (m *Module) signInWithMicrosoft(ctx context.Context, a MicrosoftAccount, id entra.Identity) (Session, error) {
	var out Session
	err := database.InTx(ctx, m.pool, authz.NewPrincipal(a.TenantID, a.UserID, "", "", ""), func(tx pgx.Tx) error {
		q := store.New(tx)
		s, err := q.MicrosoftSettings(ctx)
		if err != nil {
			return err
		}
		if !s.MicrosoftSignIn || s.MicrosoftTenantID == nil || *s.MicrosoftTenantID != id.TenantID { // changed meanwhile
			return microsoftFailed(ReasonFailed)
		}
		u, err := q.UserAuth(ctx, a.UserID)
		if err != nil {
			return err
		}
		if !u.Active && !u.Invited { // deactivated meanwhile
			return microsoftFailed(ReasonNotOnTeam)
		}
		if a.Linked {
			err = q.UsedMicrosoft(ctx, u.ID)
		} else {
			how := linkedBySignIn
			if u.Invited {
				how = linkedByInvitation
			}
			err = m.linkMicrosoft(ctx, tx, q, u, id, how)
			if errors.Is(err, errLinkTaken) || errors.Is(err, errAlreadyLinked) {
				err = microsoftFailed(ReasonFailed) // linked by another sign-in at the same moment
			}
		}
		if err != nil {
			return err
		}
		if u.Invited {
			if err := m.joinWithMicrosoft(ctx, tx, q, u.ID); err != nil {
				return err
			}
		}
		if err := q.SignedIn(ctx, u.ID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "user.signed_in", EntityType: "user", EntityID: u.ID,
			Changes: map[string]any{"method": "microsoft"}}); err != nil {
			return err
		}
		now := time.Now().Truncate(time.Second)
		for _, after := range []*time.Time{u.TokensValidAfter, u.MicrosoftSessionsAfter} {
			if after != nil && now.Before(*after) {
				now = *after
			}
		}
		out, err = m.session(a.TenantID, u.ID, authn.MethodMicrosoft, now)
		return err
	})
	return out, err
}

// joinWithMicrosoft accepts an invitation (no password is chosen).
func (m *Module) joinWithMicrosoft(ctx context.Context, tx pgx.Tx, q *store.Queries, userID string) error {
	if err := q.JoinWithMicrosoft(ctx, userID); err != nil {
		return err
	}
	if err := q.EndLinks(ctx, store.EndLinksParams{UserID: userID, Kind: "invite"}); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "user.joined", EntityType: "user", EntityID: userID, Changes: map[string]any{"method": "microsoft"}})
}
