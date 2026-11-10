// Package intake imports CVs and LinkedIn PDFs, one or many at once, as
// candidates: each file becomes a candidate named after it and is read like
// any CV; when reading ends the candidate is filled in from it, or merged
// into the candidate who already has its email (one candidate per email).
//
// Layout (the same in every module):
//
//	intake.go     the module's public API and what it needs from other modules
//	domain.go     the rules, as pure functions (unit tested)
//	service.go    use cases: one transaction each
//	http.go       HTTP handlers: contract types <-> service calls
//	internal/store generated SQL (db/queries/intake.sql); private to this module
package intake

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
)

// Deps is what the module needs from other modules, in the caller's
// transaction.
type Deps struct {
	Candidates interface {
		CreatePlaceholder(ctx context.Context, tx pgx.Tx, fileName, source string, owner *string) (string, error)
		// FillFromCV fills a candidate in from a read CV; emailOwner is the
		// candidate who already has the CV's email ("" if nobody).
		FillFromCV(ctx context.Context, tx pgx.Tx, id string, cv api.ParsedProfile) (emailOwner string, err error)
		MergeInto(ctx context.Context, tx pgx.Tx, keepID, dupID string) error
		// Names are the candidates the user may see (others are left out).
		Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
	}
	Documents interface {
		// IsCVFile reports whether data is a file CVs come in (PDF, Word, text,
		// RTF, ODT or a picture) that can be read.
		IsCVFile(fileName string, data []byte) bool
		AddFileAs(ctx context.Context, tx pgx.Tx, tenantID, candidateID, source, fileName string, data []byte) (string, func(), error)
		// SameCV is a CV with exactly data's content already in HiringPlug:
		// whose (candidateID), their name and its file name; found false: none.
		SameCV(ctx context.Context, tx pgx.Tx, data []byte) (candidateID, candidateName, fileName string, found bool, err error)
	}
	// Sources checks a candidate source is in the company's list.
	Sources interface {
		HasOption(ctx context.Context, tx pgx.Tx, list, key string) (bool, error)
	}
}

// Module is the intake module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module. Files may be at most maxUploadBytes.
func New(pool *pgxpool.Pool, deps Deps, maxUploadBytes int64) *Module {
	svc := &Service{pool: pool, deps: deps}
	return &Module{service: svc, HTTP: &HTTP{svc: svc, maxUploadBytes: maxUploadBytes}}
}

// ReadDone fills in or merges the candidate of an imported file once its
// reading has ended (documents' ReadDoneHook); other documents are ignored.
func (m *Module) ReadDone(ctx context.Context, tx pgx.Tx, tenantID, documentID string, profile *api.ParsedProfile) error {
	return m.service.readDone(ctx, tx, documentID, profile)
}
