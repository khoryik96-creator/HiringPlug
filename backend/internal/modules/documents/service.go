package documents

import (
	"context"
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
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/salaryview"
)

// downloadLinkTTL is how long the AI worker's download link stays valid. It
// covers the job waiting in the queue and its retries.
const downloadLinkTTL = 7 * 24 * time.Hour

// Service holds the document use cases.
type Service struct {
	pool           *pgxpool.Pool
	files          *blob.Store
	candidates     Candidates
	whenRead       []TextReadHook
	whenDone       []ReadDoneHook
	whenBinned     []CVBinnedHook
	enrichIndustry func(context.Context, pgx.Tx, *api.ParsedProfile) error
	industryLabels func(context.Context, pgx.Tx) ([]string, error)
	recordBinned   func(context.Context, pgx.Tx, string, string, string, string) error
	ai             AIAllowance         // nil: the AI never fills in CV readings
	salaryCVs      SalaryCVs           // formatted CVs made with the salary (CheckSalaryCVsWith)
	salaryViews    salaryview.Recorder // who was shown a salary (RecordSalaryViewsWith)
}

// AIAllowance says whether the AI may be used now (the company has one,
// and allowance left this month), and which (nil: the server's own); and
// records each AI run's tokens in the company's AI use (aiusage).
type AIAllowance interface {
	Allowed(ctx context.Context, tx pgx.Tx, tenantID string) (*api.AiAccess, bool, error)
	RecordAI(ctx context.Context, tx pgx.Tx, tenantID, feature, sourceID, runKey string, userID *string, input, output int64) error
}

// ReadDoneHook runs when reading a document has ended, in the same
// transaction: with the details read from it (as kept), or nil when its text
// or its details could not be read (e.g. CV imports fill candidates in).
type ReadDoneHook func(ctx context.Context, tx pgx.Tx, tenantID, documentID string, profile *api.ParsedProfile) error

// TextReadHook runs when a candidate's CV (not one the app made) has been
// read, in the same transaction (e.g. CV vs job matching scores them again).
type TextReadHook func(ctx context.Context, tx pgx.Tx, tenantID, candidateID string) error

// CVBinnedHook runs when a CV a candidate sent (not one the app made) is
// moved to the Bin (binned) or back from it, in the same transaction (e.g.
// search by meaning stops using what it read of a CV in the Bin).
type CVBinnedHook func(ctx context.Context, tx pgx.Tx, tenantID, candidateID, documentID string, binned bool) error

// cvBinned runs the hooks for a document moved to or from the Bin.
func (s *Service) cvBinned(ctx context.Context, tx pgx.Tx, tenantID, candidateID, documentID, source string, binned bool) error {
	if source == "formatted" {
		return nil
	}
	for _, hook := range s.whenBinned {
		if err := hook(ctx, tx, tenantID, candidateID, documentID, binned); err != nil {
			return err
		}
	}
	return nil
}

// Upload stores a file for a candidate and queues its text extraction. The
// document row and the job are committed together: there is never a document
// that nobody will process, or a job for a document that does not exist.
func (s *Service) Upload(ctx context.Context, p authz.Principal, candidateID, source, fileName string, data []byte, replace []string) (Document, error) {
	contentType, ok := DetectContentType(fileName, data)
	if !ok {
		return Document{}, apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "Upload "+filekind.Accepted+".")
	}
	var doc Document
	var storedKey string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		// The candidate has this exact file already: never twice.
		if same, err := s.sameContent(ctx, tx, &candidateID, data); err != nil {
			return err
		} else if same != nil {
			return duplicateError(*same)
		}
		var err error
		doc, storedKey, err = s.add(ctx, tx, p.TenantID, p.UserID, candidateID, source, fileName, contentType, data, replace)
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
func (s *Service) add(ctx context.Context, tx pgx.Tx, tenantID, actorID, candidateID, source, fileName, contentType string, data []byte, replace []string) (doc Document, storedKey string, err error) {
	var read *api.DocumentTextExtractedJob
	if filekind.ReadHere(contentType) { // read before anything is stored: no text, no file
		r := readHere("", contentType, data)
		if r.Status != api.ExtractionStatusExtracted {
			return Document{}, "", apperr.New(api.ErrorCodeUNSUPPORTEDMEDIATYPE, "No readable text was found in this "+filekind.Short(contentType)+" file.")
		}
		read = &r
	}
	exists, err := s.candidates.Exists(ctx, tx, candidateID)
	if err != nil {
		return Document{}, "", err
	}
	if !exists {
		return Document{}, "", apperr.NotFound("candidate")
	}
	if err := checkCVSlots(ctx, tx, candidateID, source == "formatted", replace); err != nil {
		return Document{}, "", err
	}
	if err := s.binReplaced(ctx, tx, tenantID, actorID, replace); err != nil {
		return Document{}, "", err
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
	row, err := q.InsertDocument(ctx, store.InsertDocumentParams{
		ID: id, CandidateID: candidateID, Source: source, FileName: CleanFileName(fileName),
		ContentType: contentType, SizeBytes: int64(len(data)), Sha256: SumOf(data), StorageKey: key,
	})
	if err != nil {
		return Document{}, key, err
	}
	if read != nil {
		// A text, RTF or ODT file is read here and now: no tool to run.
		read.DocumentId = id
		if err := s.applyExtraction(ctx, tx, tenantID, *read); err != nil {
			return Document{}, key, err
		}
		if row, err = q.GetDocument(ctx, id); err != nil {
			return Document{}, key, err
		}
	} else if _, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentExtractText, api.DocumentExtractTextJob{
		DocumentId:  id,
		DownloadUrl: s.files.SignedURL(key, downloadLinkTTL),
		ContentType: contentType,
		FileName:    row.FileName,
		SizeBytes:   row.SizeBytes,
	}); err != nil {
		return Document{}, key, err
	}
	// A Word, RTF or ODT CV also gets a PDF copy, so it can be previewed as it
	// looks (formatted CVs queue theirs in AddFormatted).
	if filekind.Office(contentType) && source != "formatted" {
		if err := s.queuePdf(ctx, tx, tenantID, Document{ID: id}, key); err != nil {
			return Document{}, key, err
		}
		queued := string(api.PdfStatusQueued)
		row.PdfStatus = &queued
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
		if err := s.openCV(ctx, tx, row); err != nil { // its text holds the salary
			return err
		}
		doc = fromRow(row)
		doc.ExtractedText = row.ExtractedText
		doc.ParsedProfile = row.ParsedProfile
		return nil
	})
	return doc, err
}

// Parse reads a document again (for CVs uploaded before parsing existed, or
// after the reader improved): its text is taken out of the file again (a
// newer reader sees more, such as tables), then its details are read. A file
// whose text is read here (text, RTF, ODT) only has its details read again.
func (s *Service) Parse(ctx context.Context, p authz.Principal, id string) (Document, error) {
	var doc Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		retryText := row.Status == string(api.DocumentStatusFailed) && !filekind.ReadHere(row.ContentType)
		if !hasText(row.ExtractedText) && !retryText {
			return apperr.New(api.ErrorCodeDOCUMENTHASNOTEXT, "This document has no text to read yet.")
		}
		// Asked again while it is read (a second click, two people): the
		// reading under way answers both, never a second job.
		if active, err := jobqueue.Active(ctx, tx, readingKinds, "documentId", id); err != nil || active {
			doc = fromRow(row)
			return err
		}
		switch {
		case retryText: // its text could not be read: from the file again
			if err := s.queueExtraction(ctx, tx, p.TenantID, row); err != nil {
				return err
			}
			if row, err = q.RequeueExtraction(ctx, id); err != nil {
				return err
			}
			doc = fromRow(row)
		case filekind.ReadHere(row.ContentType):
			err = s.queueParse(ctx, tx, p.TenantID, row)
		default:
			err = s.queueExtraction(ctx, tx, p.TenantID, row)
			if err == nil {
				_, err = q.QueueParse(ctx, row.ID)
			}
		}
		if err != nil {
			return err
		}
		if !retryText {
			queued := string(api.DocumentParseStatusQueued)
			doc = fromRow(row)
			doc.ParseStatus = &queued
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "document.parse_requested", EntityType: "document", EntityID: id})
	})
	return doc, err
}

// queueExtraction takes a stored file's text out again (then its details are
// read, as after an upload), with a link signed now: one in an old job may
// have expired, or come from another server.
func (s *Service) queueExtraction(ctx context.Context, tx pgx.Tx, tenantID string, row store.Document) error {
	_, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentExtractText, api.DocumentExtractTextJob{
		DocumentId: row.ID, DownloadUrl: s.files.SignedURL(row.StorageKey, downloadLinkTTL),
		ContentType: row.ContentType, FileName: row.FileName, SizeBytes: row.SizeBytes,
	})
	return err
}

func fromRow(r store.Document) Document {
	d := Document{
		ID: r.ID, CandidateID: r.CandidateID, Source: r.Source, FileName: r.FileName,
		ContentType: r.ContentType, SizeBytes: r.SizeBytes, SHA256: r.Sha256, Status: r.Status,
		Extractor: r.Extractor, ErrorMessage: r.ErrorMessage, Warnings: r.ExtractionWarnings,
		ParseStatus: r.ParseStatus, ParseError: r.ParseError, PdfStatus: r.PdfStatus, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		ReadOutdated: r.Source != "formatted" && r.ParseStatus != nil && *r.ParseStatus == string(api.DocumentParseStatusParsed) &&
			(r.Parser == nil || *r.Parser != CVReader),
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
