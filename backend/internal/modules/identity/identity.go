// Package identity knows who is signed in: it turns a bearer token into a
// Principal (tenant, user, role, permissions) and serves GET /v1/me.
//
// Layout (the same in every module):
//
//	identity.go        the module's public API, used by internal/app and other modules
//	http.go            HTTP handlers: map contract types <-> service calls, no business rules
//	internal/store     generated SQL (db/queries/identity.sql); private to this module
package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Module is the identity module.
type Module struct {
	pool     *pgxpool.Pool
	verifier *authn.Verifier
	HTTP     *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, verifier *authn.Verifier) *Module {
	m := &Module{pool: pool, verifier: verifier}
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
		u, err := store.New(tx).GetUser(ctx, claims.Subject)
		if database.IsNoRows(err) {
			return fmt.Errorf("%w: user %s not found in tenant %s", authn.ErrInvalidToken, claims.Subject, claims.TenantID)
		}
		if err != nil {
			return err
		}
		if !u.Active {
			return fmt.Errorf("%w: user %s is deactivated", authn.ErrInvalidToken, u.ID)
		}
		principal = authz.NewPrincipal(u.TenantID, u.ID, u.Email, u.Name, api.Role(u.Role))
		return nil
	})
	return principal, err
}

// tenantName returns the display name of the principal's tenant.
func (m *Module) tenantName(ctx context.Context, p authz.Principal) (string, error) {
	var name string
	err := database.InTx(ctx, m.pool, p, func(tx pgx.Tx) error {
		t, err := store.New(tx).GetTenant(ctx, p.TenantID)
		name = t.Name
		return err
	})
	return name, err
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
