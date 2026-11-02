// Package pipeline puts candidates on jobs (applications) and moves them
// through stages: applied, screening, submitted, interview, offer, placed, or
// closed as rejected or withdrawn with a reason. The rules for moves are in
// domain.go, in one place.
//
// Layout (the same in every module):
//
//	pipeline.go     the module's public API: what internal/app and other modules may use
//	domain.go       the rules, as pure functions on plain values (unit tested)
//	service.go      use cases: one transaction each, rules + storage + audit
//	http.go         HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/pipeline.sql); private to this module
package pipeline

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CandidateInfo is what an application shows of its candidate.
type CandidateInfo struct {
	Name  string
	Title *string
}

// JobInfo is what an application shows of its job.
type JobInfo struct {
	Title     string
	CompanyID string
}

// Candidates is what this module needs from the candidates module.
type Candidates interface {
	Exists(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
	Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]CandidateInfo, error)
}

// OpenJob is an open job, for the dashboard.
type OpenJob struct {
	ID, Title, CompanyID string
}

// Jobs is what this module needs from the jobs module.
type Jobs interface {
	Exists(ctx context.Context, tx pgx.Tx, jobID string) (bool, error)
	Info(ctx context.Context, tx pgx.Tx, ids []string) (map[string]JobInfo, error)
	OpenJobs(ctx context.Context, tx pgx.Tx, limit int) ([]OpenJob, error)
}

// Companies is what this module needs from the companies module.
type Companies interface {
	Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
}

// Module is the pipeline module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. The other modules are passed in by internal/app
// (adapted to the small interfaces above), so none imports another.
func New(pool *pgxpool.Pool, candidates Candidates, jobs Jobs, companies Companies) *Module {
	svc := &Service{pool: pool, candidates: candidates, jobs: jobs, companies: companies}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}
