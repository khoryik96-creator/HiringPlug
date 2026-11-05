// Package settings holds each company's main settings: the options in its
// dropdowns, and which form fields are mandatory.
//
// Layout (the same in every module):
//
//	settings.go   the module's public API: what internal/app and other modules may use
//	domain.go     the rules, as pure functions on plain values (unit tested)
//	service.go    use cases: one transaction each, rules + storage + audit
//	http.go       HTTP handlers: contract types <-> service calls; no business rules
//	internal/store generated SQL (db/queries/settings.sql); private to this module
package settings

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/settings/internal/store"
)

// Module is the settings module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool) *Module {
	svc := &Service{pool: pool}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Mandatory returns the fields the company made mandatory on a form
// ("candidate" or "job"), in the caller's transaction.
func (m *Module) Mandatory(ctx context.Context, tx pgx.Tx, form string) ([]string, error) {
	rules, err := rulesIn(ctx, store.New(tx))
	return MandatoryOn(form, rules), err
}

// Labels returns the words for every option in the company's lists
// (list -> key -> label), read once, in the caller's transaction.
func (m *Module) Labels(ctx context.Context, tx pgx.Tx) (map[string]map[string]string, error) {
	all, err := listsIn(ctx, store.New(tx))
	out := make(map[string]map[string]string, len(all))
	for _, l := range all {
		out[l.Name] = make(map[string]string, len(l.Options))
		for _, o := range l.Options {
			out[l.Name][o.Key] = o.Label
		}
	}
	return out, err
}

// HasOption reports whether key is in one of the company's lists (hidden
// options count: records may keep them), in the caller's transaction.
func (m *Module) HasOption(ctx context.Context, tx pgx.Tx, list, key string) (bool, error) {
	all, err := listsIn(ctx, store.New(tx))
	if err != nil {
		return false, err
	}
	for _, l := range all {
		if l.Name == list {
			return slices.ContainsFunc(l.Options, func(o Option) bool { return o.Key == key }), nil
		}
	}
	return false, nil
}
