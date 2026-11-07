// Package identity knows who is signed in and who is in the team: signing in
// with email and password (Argon2id hashes; attempts limited per address and
// email), invitations and password reset links (only their SHA-256 kept),
// turning a bearer token into a Principal (tenant, user, role, permissions),
// and the team admin rules (nobody locks themselves out; the last owner
// stays). Changing a password or deactivating someone signs out their
// sessions.
//
// Layout (the same in every module):
//
//	identity.go        the module's public API, used by internal/app and other modules
//	domain.go          password and team rules, as pure functions (unit tested)
//	auth.go            signing in, links (invitations, resets), changing a password
//	team.go            team admin: invitations, roles, deactivating
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
}

// Module is the identity module.
type Module struct {
	pool      *pgxpool.Pool
	verifier  *authn.Verifier
	mailer    Mailer
	publicURL string
	limits    limits
	HTTP      *HTTP
}

// New wires the module. Links in emails point to publicURL (the web app).
func New(pool *pgxpool.Pool, verifier *authn.Verifier, mailer Mailer, publicURL string) *Module {
	m := &Module{pool: pool, verifier: verifier, mailer: mailer, publicURL: publicURL, limits: newLimits(time.Now)}
	m.HTTP = &HTTP{m: m}
	return m
}

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
			principal = principal.WithSession(authz.Session{ID: claims.ID, Expires: claims.ExpiresAt.Time})
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
	for _, u := range found {
		a, err := roleAccess(ctx, q, u.Role)
		if err != nil {
			return nil, err
		}
		out[u.ID] = a.Permissions
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
