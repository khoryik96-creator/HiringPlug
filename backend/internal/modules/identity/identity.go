// Package identity knows who is signed in and who is in the team: signing in
// with email and password (Argon2id hashes; attempts limited per address and
// email), invitations and password reset links (only their SHA-256 kept),
// turning a bearer token into a Principal (tenant, user, role, permissions),
// and the team admin rules (nobody locks themselves out; the last owner
// stays). Changing a password or deactivating someone signs out their
// sessions. People can also sign in with Microsoft (microsoft*.go).
//
// Layout (the same in every module):
//
//	identity.go        the module's public API, used by internal/app and other modules
//	domain.go          password and team rules, as pure functions (unit tested)
//	auth.go            signing in, changing a password
//	links.go           links emailed to people (invitations, resets)
//	team.go            team admin: invitations, roles, deactivating
//	microsoft*.go      signing in with Microsoft, and the company's settings for it
//	http.go, team_http.go  HTTP handlers: contract types <-> service calls
//	internal/store     generated SQL (db/queries/identity.sql); private to this module
package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Mailer sends the app's own emails (invitations, password resets), in the
// caller's transaction (emails module).
type Mailer interface {
	Notify(ctx context.Context, tx pgx.Tx, tenantID string, to []string, subject, body string) error
	// Invite sends an invitation for inviterID, who is shown its link too:
	// from the server's mail, or their own mailbox when the server's is not
	// set up. It returns how it goes (server, mailbox, test_inbox or none).
	// The email's id comes back too: the Team page says how it went.
	Invite(ctx context.Context, tx pgx.Tx, tenantID, inviterID string, to []string, subject, body string) (delivery, emailID string, err error)
}

// MailState is how an email went (emails module): queued, sent or failed, and why.
type MailState struct {
	Status string
	Error  *string
	At     time.Time
}

// InviteMail reads how invitation emails went (emails module).
type InviteMail interface {
	EmailStates(ctx context.Context, tx pgx.Tx, ids []string) (map[string]MailState, error)
}

// ReadInviteMailWith sets how invitation emails are read (called by internal/app).
func (m *Module) ReadInviteMailWith(r InviteMail) { m.inviteMail = r }

// Module is the identity module.
type Module struct {
	pool       *pgxpool.Pool
	verifier   *authn.Verifier
	mailer     Mailer
	inviteMail InviteMail // nil: the Team page does not say how invitations went
	publicURL  string
	limits     limits
	twoStep    twoStepKeys
	ms         *microsoftSignIn // nil until UseMicrosoft
	// now is the app server's clock (links, two-step codes). Sessions are
	// issued and ended by the database's clock instead (databaseSecond).
	now  func() time.Time
	HTTP *HTTP
}

// New wires the module. Links in emails point to publicURL (the web app).
func New(pool *pgxpool.Pool, verifier *authn.Verifier, mailer Mailer, publicURL string) *Module {
	m := &Module{pool: pool, verifier: verifier, mailer: mailer, publicURL: publicURL, limits: newLimits(time.Now), now: time.Now}
	m.HTTP = &HTTP{m: m}
	return m
}

// UseClock sets the app server's clock: tests only, to show that a server
// whose clock is off ends no session too late.
func (m *Module) UseClock(now func() time.Time) { m.now = now }

// Authenticate verifies a bearer token and loads the user it names. Inactive
// or unknown users are rejected even if the token is otherwise valid.
func (m *Module) Authenticate(ctx context.Context, token string) (authz.Principal, error) {
	claims, err := m.verifier.Verify(token)
	if err != nil {
		return authz.Principal{}, err
	}
	var principal authz.Principal
	err = database.InTx(ctx, m.pool, authz.TenantOnly(claims.TenantID), func(tx pgx.Tx) error {
		u, err := store.New(tx).UserAuth(ctx, claims.Subject)
		if database.IsNoRows(err) {
			return fmt.Errorf("%w: user %s not found in tenant %s", authn.ErrInvalidToken, claims.Subject, claims.TenantID)
		}
		if err != nil {
			return err
		}
		if !u.Active {
			return fmt.Errorf("%w: user %s is deactivated", authn.ErrInvalidToken, u.ID)
		}
		// Signed out by a password change or a deactivation since the token was issued.
		if u.TokensValidAfter != nil && claims.IssuedAt != nil && claims.IssuedAt.Before(*u.TokensValidAfter) {
			return fmt.Errorf("%w: token issued before the user's sessions were ended", authn.ErrInvalidToken)
		}
		// The company requires Microsoft: password sessions work only for owners.
		if PasswordRefused(u.MicrosoftRequired, u.Role) && claims.Method() != authn.MethodMicrosoft {
			return fmt.Errorf("%w: the company requires signing in with Microsoft", authn.ErrInvalidToken)
		}
		// Microsoft sessions stop when the company stops signing in with
		// Microsoft, and when the link (or the organisation) changed since.
		if claims.Method() == authn.MethodMicrosoft {
			if !u.MicrosoftSignIn {
				return fmt.Errorf("%w: the company no longer signs in with Microsoft", authn.ErrInvalidToken)
			}
			if u.MicrosoftSessionsAfter != nil && claims.IssuedAt != nil && claims.IssuedAt.Before(*u.MicrosoftSessionsAfter) {
				return fmt.Errorf("%w: Microsoft session from before the account was unlinked", authn.ErrInvalidToken)
			}
		}
		// Signed out on the server (this token only).
		if claims.ID != "" {
			revoked, err := store.New(tx).SessionRevoked(ctx, claims.ID)
			if err != nil {
				return err
			}
			if revoked {
				return fmt.Errorf("%w: signed out", authn.ErrInvalidToken)
			}
		}
		// Their company's settings for their role: permissions, and which candidates they see.
		if principal, err = principalOf(ctx, store.New(tx), u); err != nil {
			return err
		}
		if claims.ID != "" && claims.ExpiresAt != nil {
			principal = principal.WithSession(authz.Session{ID: claims.ID, Expires: claims.ExpiresAt.Time, Method: claims.Method()})
		}
		return nil
	})
	return principal, err
}

// tenant returns the principal's tenant: its name and careers page address.
func (m *Module) tenant(ctx context.Context, p authz.Principal) (store.GetTenantRow, error) {
	var t store.GetTenantRow
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		var err error
		t, err = store.New(tx).GetTenant(ctx, p.TenantID)
		return err
	})
	return t, err
}

// CareersTenant finds the company behind a public careers page address. It
// runs outside any tenant scope: the database function returns only the
// company's id and name. ok is false for an unknown address.
func (m *Module) CareersTenant(ctx context.Context, slug string) (tenantID, name string, ok bool, err error) {
	row, err := store.New(m.pool).CareersTenant(ctx, slug)
	if database.IsNoRows(err) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return row.TenantID, row.TenantName, true, nil
}

// UserNames returns the names of the given users (missing ones are left out),
// for other modules that show who did something.
func (m *Module) UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	rows, err := store.New(tx).UserNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// ActiveUsers returns, for those of the given users who are active in the
// caller's tenant, what their role may do there (the company's settings for
// it), e.g. to check interviewers and owners, in the caller's transaction.
func (m *Module) ActiveUsers(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]api.Permission, error) {
	q := store.New(tx)
	found, err := q.ActiveUserRoles(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]api.Permission, len(found))
	byRole := map[string][]api.Permission{} // each role's access is read once
	for _, u := range found {
		perms, ok := byRole[u.Role]
		if !ok {
			a, err := roleAccess(ctx, q, u.Role)
			if err != nil {
				return nil, err
			}
			perms, byRole[u.Role] = a.Permissions, a.Permissions
		}
		out[u.ID] = perms
	}
	return out, nil
}

// TenantName returns the company's name, in the caller's transaction.
func (m *Module) TenantName(ctx context.Context, tx pgx.Tx, tenantID string) (string, error) {
	t, err := store.New(tx).GetTenant(ctx, tenantID)
	return t.Name, err
}

// Contact is an active user's name and email address.
type Contact struct{ Name, Email string }

// ActiveContacts returns the names and addresses of those of the given users
// who are active (to email interviewers), in the caller's transaction.
func (m *Module) ActiveContacts(ctx context.Context, tx pgx.Tx, ids []string) (map[string]Contact, error) {
	rows, err := store.New(tx).ActiveUserEmails(ctx, ids)
	out := make(map[string]Contact, len(rows))
	for _, r := range rows {
		out[r.ID] = Contact{Name: r.Name, Email: r.Email}
	}
	return out, err
}

// ActiveMember is an active user's ID and name.
type ActiveMember struct{ ID, Name string }

// ActiveMembers returns the company's active users, by name, in the caller's
// transaction.
func (m *Module) ActiveMembers(ctx context.Context, tx pgx.Tx) ([]ActiveMember, error) {
	rows, err := store.New(tx).ListTeam(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ActiveMember, 0, len(rows))
	for _, r := range rows {
		if r.Active {
			out = append(out, ActiveMember{ID: r.ID, Name: r.Name})
		}
	}
	return out, nil
}
