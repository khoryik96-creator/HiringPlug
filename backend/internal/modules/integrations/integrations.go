// Package integrations connects other systems: API keys that use this API as
// a role (never as owner), and webhooks that POST a signed JSON message to an
// HTTPS address when chosen events happen.
//
// Only the SHA-256 of a key is kept; the key is shown once. A webhook's
// deliveries come from the audit log: a database trigger turns every change a
// webhook listens for into a delivery and queues it, in the same transaction
// as the change. The Go worker posts it (never to the server's own or private
// networks, never following redirects) and tries again later when the
// receiver fails. API keys cannot manage API keys or webhooks.
//
// Layout (the same in every module):
//
//	integrations.go  the module's public API (signing in with a key)
//	domain.go        the rules, as pure functions on plain values (unit tested)
//	service.go       use cases: one transaction each
//	jobs.go          webhook.deliver: posting a delivery
//	http.go          HTTP handlers: contract types <-> service calls
//	internal/store   generated SQL (db/queries/integrations.sql); private to this module
package integrations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/integrations/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authn"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Users names who created API keys, and says what a role may do in the
// company (identity module).
type Users interface {
	UserNames(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	RolePermissions(ctx context.Context, tx pgx.Tx, role string) ([]api.Permission, error)
}

// Module is the integrations module.
type Module struct {
	pool    *pgxpool.Pool
	service *Service
	HTTP    *HTTP
}

// New wires the module. allowPrivate lets webhooks reach private networks and
// plain HTTP addresses (local testing only).
func New(pool *pgxpool.Pool, users Users, allowPrivate bool) *Module {
	svc := &Service{pool: pool, users: users, allowPrivate: allowPrivate}
	return &Module{pool: pool, service: svc, HTTP: &HTTP{svc: svc}}
}

// Authenticate signs in with an API key: the principal is the key, with what
// its role may do in its company (the company's settings for the role), and
// every candidate (a key owns none). Unknown and revoked keys are refused.
func (m *Module) Authenticate(ctx context.Context, key string) (authz.Principal, error) {
	row, err := store.New(m.pool).ApiKeyByHash(ctx, HashKey(key))
	if database.IsNoRows(err) {
		return authz.Principal{}, fmt.Errorf("%w: unknown or revoked API key", authn.ErrInvalidToken)
	}
	if err != nil {
		return authz.Principal{}, err
	}
	var perms []api.Permission
	err = database.InTx(ctx, m.pool, authz.TenantOnly(row.TenantID), func(tx pgx.Tx) error {
		perms, err = m.service.users.RolePermissions(ctx, tx, row.KeyRole)
		return err
	})
	if err != nil {
		return authz.Principal{}, err
	}
	return authz.NewPrincipalWithAccess(row.TenantID, row.KeyID, "", "API key: "+row.KeyName, api.Role(row.KeyRole), perms, authz.Scope{}), nil
}

// JobHandlers returns the module's handlers for the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindWebhookDeliver: m.service.deliver}
}
