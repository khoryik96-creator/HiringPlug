package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/entra"
)

// MicrosoftSignIn is a company's sign-in with Microsoft, as Settings shows it.
type MicrosoftSignIn struct {
	Enabled        bool
	OrganisationID *string
	Required       bool
	CallbackURL    string
	AppSource      string // agency, server or none
	ClientID       *string
	// AcctMissing: the last Microsoft sign-in came without the optional acct
	// claim, so no account was linked by its address (Settings warns).
	AcctMissing bool
}

// MicrosoftSignInSettings returns the company's settings.
func (m *Module) MicrosoftSignInSettings(ctx context.Context, p authz.Principal) (MicrosoftSignIn, error) {
	var out MicrosoftSignIn
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		var err error
		out, err = m.microsoftSignIn(ctx, tx, store.New(tx), p.TenantID)
		return err
	})
	return out, err
}

func (m *Module) microsoftSignIn(ctx context.Context, tx pgx.Tx, q *store.Queries, tenantID string) (MicrosoftSignIn, error) {
	s, err := q.MicrosoftSettings(ctx)
	if err != nil {
		return MicrosoftSignIn{}, err
	}
	out := MicrosoftSignIn{Enabled: s.MicrosoftSignIn, OrganisationID: s.MicrosoftTenantID, Required: s.MicrosoftRequired,
		CallbackURL: m.microsoftCallback(), AppSource: "none", AcctMissing: s.MicrosoftSignIn && s.MicrosoftAcctMissing}
	if m.ms == nil {
		return out, nil
	}
	app, err := m.ms.apps.MicrosoftSignInApp(ctx, tx, tenantID)
	if err != nil {
		return out, err
	}
	if app.usable() {
		out.AppSource, out.ClientID = "server", &app.ClientID
		if app.Agency {
			out.AppSource = "agency"
		}
	}
	return out, nil
}

// SetMicrosoftSignIn lets the team sign in with Microsoft accounts of the
// company's organisation (or stops it), and requires it of everyone but
// owners (password sessions of the others stop working at once: see
// Authenticate). Turning it off ends the sessions signed in with Microsoft
// and every reconnect link sent; changing the organisation ends them too
// and removes the links to the old organisation's accounts (counted in the
// audit entry): everyone links an account of the new one at their next
// sign-in (owners from Your account).
func (m *Module) SetMicrosoftSignIn(ctx context.Context, p authz.Principal, enabled bool, organisation *string, required bool) (MicrosoftSignIn, error) {
	var org *string
	if organisation != nil && *organisation != "" {
		id := entra.NormalizeID(*organisation)
		org = &id
	}
	var errs apperr.FieldErrors
	switch {
	case org != nil && *org == entra.ConsumersTenant:
		errs.Add("organisationId", "is the organisation of personal Microsoft accounts: enter your company's Directory (tenant) ID")
	case org != nil && !entra.IsOrganisation(*org):
		errs.Add("organisationId", "enter the Directory (tenant) ID shown on the app's overview page in Microsoft Entra, e.g. 72f988bf-86f1-41af-91ab-2d7cd011db47")
	case enabled && org == nil:
		errs.Add("organisationId", "is needed to let the team sign in with Microsoft: only its accounts are accepted")
	}
	if required && !enabled {
		errs.Add("required", "turn signing in with Microsoft on first")
	}
	if err := errs.Err(); err != nil {
		return MicrosoftSignIn{}, err
	}
	var out MicrosoftSignIn
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		// First: a change of the Microsoft app (emails module) under way
		// finishes before the app is looked at, and none starts meanwhile.
		if err := q.LockMicrosoftSettings(ctx); err != nil {
			return err
		}
		before, err := m.microsoftSignIn(ctx, tx, q, p.TenantID)
		if err != nil {
			return err
		}
		if enabled && before.AppSource == "none" {
			return apperr.Validation(apperr.FieldErrors{{Field: "enabled",
				Message: "set up the Microsoft app first (Settings > Email connection), or ask the server's administrator to"}})
		}
		if _, err := q.SetMicrosoftSettings(ctx, store.SetMicrosoftSettingsParams{Enabled: enabled, MicrosoftTenantID: org, Required: required}); err != nil {
			return err
		}
		if out, err = m.microsoftSignIn(ctx, tx, q, p.TenantID); err != nil {
			return err
		}
		orgChanged := deref(before.OrganisationID) != deref(out.OrganisationID)
		if before.Enabled == out.Enabled && before.Required == out.Required && !orgChanged {
			return nil
		}
		changes := map[string]any{"enabled": enabled, "organisationId": org, "required": required,
			"before": map[string]any{"enabled": before.Enabled, "organisationId": before.OrganisationID, "required": before.Required}}
		if (before.Enabled && !out.Enabled) || (orgChanged && before.OrganisationID != nil) {
			// Everyone's row first, then their links (the order a sign-in
			// and an unlink take them in): the two wait, never deadlock.
			if err := q.HoldTeam(ctx); err != nil {
				return err
			}
		}
		if orgChanged && before.OrganisationID != nil {
			// Accounts of the old organisation no longer sign anyone in:
			// their links go, and everyone links again at their next sign-in.
			removed, err := q.UnlinkOtherOrganisations(ctx, org)
			if err != nil {
				return err
			}
			changes["linksRemoved"] = removed
		}
		if (before.Enabled && !out.Enabled) || (orgChanged && before.OrganisationID != nil) {
			if err := q.EndMicrosoftSessions(ctx, nil); err != nil {
				return err
			}
			// A reconnect link was sent for an account of the organisation
			// then: it links none from now on, even if turned on again.
			if err := q.EndLinksOfKind(ctx, "reconnect"); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "agency.microsoft_sign_in_changed", EntityType: "tenant", EntityID: p.TenantID, Changes: changes})
	})
	return out, err
}

// SignInMethods is how each team member can sign in (for the team list):
// "password", "microsoft", both or none.
func (m *Module) SignInMethods(ctx context.Context, p authz.Principal) (map[string][]string, error) {
	out := map[string][]string{}
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).SignInMethods(ctx)
		for _, r := range rows {
			methods := []string{}
			if r.HasPassword {
				methods = append(methods, "password")
			}
			if r.Microsoft {
				methods = append(methods, "microsoft")
			}
			out[r.ID] = methods
		}
		return err
	})
	return out, err
}

// provePerson checks it is really them before two-step sign-in is turned
// off: their password, or (they sign in with Microsoft and have none) the
// authenticator app's current code.
func (m *Module) provePerson(ctx context.Context, q *store.Queries, u store.UserAuthRow, password, code *string) error {
	if u.PasswordHash != nil {
		if password == nil || !CheckPassword(*u.PasswordHash, *password) {
			return apperr.Validation(apperr.FieldErrors{{Field: "password", Message: "is not your current password"}})
		}
		return nil
	}
	if code == nil {
		return apperr.Validation(apperr.FieldErrors{{Field: "code", Message: "you sign in with Microsoft and have no password: enter the code your authenticator app shows now"}})
	}
	if _, err := m.checkCode(ctx, q, u.TenantID, u.ID, *code, false); err != nil {
		return apperr.Validation(apperr.FieldErrors{{Field: "code", Message: "is not the code the app shows now"}})
	}
	return nil
}
