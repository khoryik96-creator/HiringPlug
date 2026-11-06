// Package aiproviders holds the AI provider each company uses (DeepSeek,
// Claude or ChatGPT with its own API key, the server's own, or none).
//
// Layout (the same in every module):
//
//	aiproviders.go  the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases: one transaction each, rules + storage + audit
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/aiproviders.sql); private to this module
package aiproviders

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Settings is how the server is set up.
type Settings struct {
	ServerOn   bool   // the server has its own provider (HP_AI_PROVIDER)
	KeysSecret []byte // seals companies' keys (HP_AI_KEYS_SECRET); empty: none can be saved
}

// Module is the aiproviders module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, settings Settings) *Module {
	svc := &Service{pool: pool, settings: settings}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Access returns the AI the company uses, in the caller's transaction: on
// is false when it has none; access is nil when it uses the server's own
// provider. Modules put access on the AI jobs they queue.
func (m *Module) Access(ctx context.Context, tx pgx.Tx) (access *api.AiAccess, on bool, err error) {
	return m.service.access(ctx, tx)
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{api.JobKindAiproviderChecked: m.service.checked}
}
