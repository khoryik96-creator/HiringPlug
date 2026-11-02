// Package documents stores files attached to candidates (CVs) and their
// extracted text.
//
// Uploading a CV stores the file, records it, and queues a
// document.extract_text job for the Python AI worker, all in one
// transaction. The worker answers with a document.text_extracted job, which
// this module's handler saves on the document.
//
// Layout (the same in every module):
//
//	documents.go     the module's public API: what internal/app and other modules may use
//	domain.go        the rules, as pure functions on plain values (unit tested)
//	service.go       use cases: one transaction each
//	jobs.go          handlers for this module's jobs on the core queue
//	http.go          HTTP handlers: contract types <-> service calls
//	internal/store   generated SQL (db/queries/documents.sql); private to this module
package documents

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Candidates is what this module needs from the candidates module. Defining
// it here (rather than importing that module's types) keeps the dependency
// small and lets tests substitute it.
type Candidates interface {
	Exists(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
}

// Module is the documents module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, files *blob.Store, candidates Candidates, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, files: files, candidates: candidates}
	return &Module{service: svc, HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{
		api.JobKindDocumentTextExtracted: m.service.saveExtraction,
		api.JobKindDocumentCvParsed:      m.service.saveParse,
	}
}
