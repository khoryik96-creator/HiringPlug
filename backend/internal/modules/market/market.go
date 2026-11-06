// Package market is a job's market map: who in the company's own database
// does the job's kind of work (how many, how available, where they work and
// live, their work authorization, notice and, for roles that may see
// salaries, their pay), with the AI assistant's market insights, exported to
// Word or PDF. It keeps no data of its own.
//
// Layout (the same in every module):
//
//	market.go   the module's public API and what it needs from other modules
//	domain.go   the pool and the document (unit tested)
//	service.go  use cases: one transaction each
//	http.go     HTTP handlers: contract types <-> service calls
package market

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Deps is what the module needs from other modules, in the caller's transaction.
type Deps struct {
	// Jobs returns a job's title; a NotFound error for a job p may not see.
	Jobs interface {
		Title(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (string, error)
	}
	Candidates interface {
		Members(ctx context.Context, tx pgx.Tx, p authz.Principal, words string) ([]Member, []string, error)
		Pay(ctx context.Context, tx pgx.Tx, p authz.Principal, ids []string) ([]Pay, error)
	}
	// Insights returns the user's newest market insights for a job (nil if none).
	Insights interface {
		Latest(ctx context.Context, tx pgx.Tx, jobID string) (*Answer, error)
	}
}

// Answer is the AI assistant's market insights for a job.
type Answer struct {
	ID, Status  string
	Text, Model *string
	AskedAt     time.Time
}

// Module is the market module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, deps Deps) *Module {
	svc := &Service{pool: pool, deps: deps, now: time.Now}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// PoolText describes who in the database does a job's kind of work (counts
// only: no names, contact details or pay), for the AI's market insights.
func (m *Module) PoolText(ctx context.Context, tx pgx.Tx, p authz.Principal, jobID string) (string, error) {
	title, err := m.service.deps.Jobs.Title(ctx, tx, p, jobID)
	if err != nil {
		return "", err
	}
	words := TitleWords(title)
	members, _, err := m.service.deps.Candidates.Members(ctx, tx, p, words)
	if err != nil {
		return "", err
	}
	return PoolText(words, Build(members, nil)), nil
}
