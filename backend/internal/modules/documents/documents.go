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
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// Candidates is what this module needs from the candidates module. Defining
// it here (rather than importing that module's types) keeps the dependency
// small and lets tests substitute it.
type Candidates interface {
	Exists(ctx context.Context, tx pgx.Tx, candidateID string) (bool, error)
	Names(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error)
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

// AddFile stores a CV for a candidate and queues reading it, in the caller's
// transaction (the careers page's apply form). The file type is checked from
// its bytes. undo removes the stored file; call it if the transaction rolls
// back.
func (m *Module) AddFile(ctx context.Context, tx pgx.Tx, tenantID, candidateID, fileName string, data []byte) (documentID string, undo func(), err error) {
	return m.AddFileAs(ctx, tx, tenantID, candidateID, "original", fileName, data)
}

// AddFileAs is AddFile with the document's source (original, linkedin, ...).
// SameCV is a CV with exactly data's content already in the tenant (tx's):
// whose it is, their name and its file name. Imports skip such a file.
func (m *Module) SameCV(ctx context.Context, tx pgx.Tx, data []byte) (candidateID, candidateName, fileName string, found bool, err error) {
	match, err := m.service.sameContent(ctx, tx, nil, data)
	if err != nil || match == nil {
		return "", "", "", false, err
	}
	return match.CandidateID, match.CandidateName, match.FileName, true, nil
}

func (m *Module) AddFileAs(ctx context.Context, tx pgx.Tx, tenantID, candidateID, source, fileName string, data []byte) (documentID string, undo func(), err error) {
	contentType, ok := DetectContentType(fileName, data)
	if !ok {
		return "", func() {}, apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "Upload "+filekind.Accepted+".")
	}
	// The candidate has this exact file already (e.g. applying again with
	// the same CV): it is theirs once, and that one is used.
	same, err := m.service.sameContent(ctx, tx, &candidateID, data)
	if err != nil {
		return "", func() {}, err
	}
	if same != nil {
		return same.DocumentID, func() {}, nil
	}
	doc, key, err := m.service.add(ctx, tx, tenantID, candidateID, source, fileName, contentType, data)
	undo = func() {
		if key != "" {
			_ = m.service.files.Delete(key)
		}
	}
	return doc.ID, undo, err
}

// AddFormatted stores a formatted CV made by the app (a .docx) for a
// candidate, in the caller's transaction; it is read and parsed like any CV,
// so it can be compared with the original, and turned into a PDF. undo removes the stored file;
// call it if the transaction rolls back.
func (m *Module) AddFormatted(ctx context.Context, tx pgx.Tx, tenantID, candidateID, fileName string, data []byte) (documentID string, undo func(), err error) {
	doc, key, err := m.service.add(ctx, tx, tenantID, candidateID, "formatted", fileName,
		ContentTypeDOCX, data)
	undo = func() {
		if key != "" {
			_ = m.service.files.Delete(key)
		}
	}
	if err == nil {
		err = m.service.queuePdf(ctx, tx, tenantID, doc, key) // and as a PDF
	}
	return doc.ID, undo, err
}

// LatestCVText returns the text of the candidate's newest CV that was read
// (uploaded, not made by the app), or "" when there is none, in the caller's
// transaction.
func (m *Module) LatestCVText(ctx context.Context, tx pgx.Tx, candidateID string) (string, error) {
	text, err := store.New(tx).LatestCvText(ctx, candidateID)
	if database.IsNoRows(err) {
		return "", nil
	}
	return text, err
}

// JobHandlers returns the handlers for this module's jobs on the core queue.
func (m *Module) JobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{
		api.JobKindDocumentTextExtracted: m.service.saveExtraction,
		api.JobKindDocumentCvParsed:      m.service.saveParse,
		api.JobKindDocumentPdfRendered:   m.service.savePdf,
	}
}

// UseIndustryRecognition enriches grounded employer evidence using the shared Go rules.
func (m *Module) UseIndustryRecognition(f func(context.Context, pgx.Tx, *api.ParsedProfile) error) {
	m.service.enrichIndustry = f
}
