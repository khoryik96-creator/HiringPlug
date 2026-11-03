package documents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/blob"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
)

// downloadLinkTTL is how long the AI worker's download link stays valid. It
// covers the job waiting in the queue and its retries.
const downloadLinkTTL = 7 * 24 * time.Hour

// Service holds the document use cases.
type Service struct {
	pool       *pgxpool.Pool
	files      *blob.Store
	candidates Candidates
}

// Upload stores a file for a candidate and queues its text extraction. The
// document row and the job are committed together: there is never a document
// that nobody will process, or a job for a document that does not exist.
func (s *Service) Upload(ctx context.Context, p authz.Principal, candidateID, source, fileName string, data []byte) (Document, error) {
	contentType, ok := DetectContentType(data)
	if !ok {
		return Document{}, apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "Upload a PDF or a Word (.docx or .doc) file.")
	}
	var doc Document
	var storedKey string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		doc, storedKey, err = s.add(ctx, tx, p.TenantID, candidateID, source, fileName, contentType, data)
		return err
	})
	if err != nil && storedKey != "" {
		// The transaction rolled back, so nothing refers to the file.
		err = errors.Join(err, s.files.Delete(storedKey))
	}
	return doc, err
}

// add stores a file for a candidate, records it and queues its text
// extraction, in the caller's transaction. storedKey is set once the file is
// written, so the caller can remove it if the transaction rolls back.
func (s *Service) add(ctx context.Context, tx pgx.Tx, tenantID, candidateID, source, fileName, contentType string, data []byte) (doc Document, storedKey string, err error) {
	exists, err := s.candidates.Exists(ctx, tx, candidateID)
	if err != nil {
		return Document{}, "", err
	}
	if !exists {
		return Document{}, "", apperr.NotFound("candidate")
	}
	q := store.New(tx)
	id, err := q.NewDocumentID(ctx)
	if err != nil {
		return Document{}, "", err
	}
	key := tenantID + "/" + id
	if err := s.files.Put(key, data); err != nil {
		return Document{}, "", err
	}
	sum := sha256.Sum256(data)
	row, err := q.InsertDocument(ctx, store.InsertDocumentParams{
		ID: id, CandidateID: candidateID, Source: source, FileName: CleanFileName(fileName),
		ContentType: contentType, SizeBytes: int64(len(data)), Sha256: hex.EncodeToString(sum[:]), StorageKey: key,
	})
	if err != nil {
		return Document{}, key, err
	}
	if _, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentExtractText, api.DocumentExtractTextJob{
		DocumentId:  id,
		DownloadUrl: s.files.SignedURL(key, downloadLinkTTL),
		ContentType: contentType,
		FileName:    row.FileName,
		SizeBytes:   row.SizeBytes,
	}); err != nil {
		return Document{}, key, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{
		Action: "document.uploaded", EntityType: "document", EntityID: id,
		Changes: map[string]any{"candidateId": candidateID, "source": source, "fileName": row.FileName},
	}); err != nil {
		return Document{}, key, err
	}
	return fromRow(row), key, nil
}

// List returns a candidate's documents, newest first, without their text.
func (s *Service) List(ctx context.Context, p authz.Principal, candidateID string) ([]Document, error) {
	var docs []Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		exists, err := s.candidates.Exists(ctx, tx, candidateID)
		if err != nil {
			return err
		}
		if !exists {
			return apperr.NotFound("candidate")
		}
		rows, err := store.New(tx).ListDocumentsForCandidate(ctx, candidateID)
		if err != nil {
			return err
		}
		docs = make([]Document, len(rows))
		for i, r := range rows {
			docs[i] = fromRow(store.Document(r))
		}
		return nil
	})
	return docs, err
}

// Get returns one document, including its extracted text.
func (s *Service) Get(ctx context.Context, p authz.Principal, id string) (Document, error) {
	var doc Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		doc = fromRow(row)
		doc.ExtractedText = row.ExtractedText
		doc.ParsedProfile = row.ParsedProfile
		return nil
	})
	return doc, err
}

// Parse queues parsing of a document's text again (for CVs uploaded before
// parsing existed, or after the parser improved).
func (s *Service) Parse(ctx context.Context, p authz.Principal, id string) (Document, error) {
	var doc Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		row, err := store.New(tx).GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		if !hasText(row.ExtractedText) {
			return apperr.New(api.ErrorCodeDOCUMENTHASNOTEXT, "This document has no text to read yet.")
		}
		if err := queueParse(ctx, tx, p.TenantID, row); err != nil {
			return err
		}
		queued := string(api.DocumentParseStatusQueued)
		doc = fromRow(row)
		doc.ParseStatus = &queued
		return audit.Record(ctx, tx, audit.Entry{Action: "document.parse_requested", EntityType: "document", EntityID: id})
	})
	return doc, err
}

func fromRow(r store.Document) Document {
	d := Document{
		ID: r.ID, CandidateID: r.CandidateID, Source: r.Source, FileName: r.FileName,
		ContentType: r.ContentType, SizeBytes: r.SizeBytes, SHA256: r.Sha256, Status: r.Status,
		Extractor: r.Extractor, ErrorMessage: r.ErrorMessage, Warnings: r.ExtractionWarnings,
		ParseStatus: r.ParseStatus, PdfStatus: r.PdfStatus, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
	if r.PageCount != nil {
		n := int(*r.PageCount)
		d.PageCount = &n
	}
	return d
}
