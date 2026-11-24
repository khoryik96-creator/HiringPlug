package documents

// Word documents also come as a PDF (formatted CVs to send, CVs sent as Word
// to preview): the Python worker turns the Word file into one with
// LibreOffice, and the Go worker saves it with the document.

import (
	"context"
	"io"
	"mime"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/pdfcopy"
)

// queuePdf asks the Python worker for a document's PDF.
func (s *Service) queuePdf(ctx context.Context, tx pgx.Tx, tenantID string, doc Document, key string) error {
	if err := store.New(tx).QueuePdf(ctx, doc.ID); err != nil {
		return err
	}
	_, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentRenderPdf, api.DocumentRenderPdfJob{
		DocumentId: doc.ID, DownloadUrl: s.files.SignedURL(key, downloadLinkTTL),
	})
	return err
}

// savePdf handles document.pdf_rendered: the PDF, or why it failed.
func (s *Service) savePdf(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.DocumentPdfRenderedJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	saved, err := pdfcopy.Save(s.files, job.TenantID, result.DocumentId, job.ID, result.PdfBase64, result.Error)
	if err != nil {
		return err
	}
	_, err = store.New(tx).SavePdf(ctx, store.SavePdfParams{ID: result.DocumentId, PdfStatus: &saved.Status, PdfKey: saved.Key, PdfError: saved.Error})
	if database.IsNoRows(err) {
		// Gone, or not waited for: only this result's own file goes.
		logx.From(ctx).Info("document gone or its PDF not waited for; dropping it", "document_id", result.DocumentId)
		if saved.Key != nil {
			_ = s.files.Delete(*saved.Key)
		}
		return nil
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "document.pdf_rendered", EntityType: "document", EntityID: result.DocumentId, Changes: map[string]any{"status": saved.Status}})
}

// Pdf opens a document's PDF.
func (s *Service) Pdf(ctx context.Context, p authz.Principal, id string) (Document, io.ReadCloser, error) {
	var row store.Document
	var f io.ReadCloser
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		closeFile(&f) // a transaction tried again
		var err error
		row, err = store.New(tx).GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		if row.PdfKey == nil { // nothing to open: only refused to those who may not see it
			if _, err := s.salaryCV(ctx, tx, row); err != nil {
				return err
			}
			return apperr.NotFound("PDF of this document")
		}
		f, err = s.openFile(ctx, tx, row, *row.PdfKey, "PDF of this document")
		return err
	})
	if err != nil {
		closeFile(&f)
		return Document{}, nil, err
	}
	return fromRow(row), f, nil
}

// GetDocumentPdf handles GET /v1/documents/{documentId}/pdf.
func (h *HTTP) GetDocumentPdf(ctx context.Context, req api.GetDocumentPdfRequestObject) (api.GetDocumentPdfResponseObject, error) {
	doc, f, err := h.svc.Pdf(ctx, authz.MustFromContext(ctx), req.DocumentId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": pdfcopy.FileName(doc.FileName)})
	return api.GetDocumentPdf200ApplicationoctetStreamResponse{
		Body: f, Headers: api.GetDocumentPdf200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

// MakePdf queues a Word document's PDF copy when it has none (one uploaded
// before previews began); anything else is returned as it is.
func (s *Service) MakePdf(ctx context.Context, p authz.Principal, id string) (Document, error) {
	var out Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		if _, err := s.salaryCV(ctx, tx, row); err != nil {
			return err
		}
		if filekind.Office(row.ContentType) && row.PdfStatus == nil { // a failed copy is not tried again on every view
			key, err := q.QueuePdfIfNone(ctx, id) // two asking at once: one queues it
			switch {
			case database.IsNoRows(err):
			case err != nil:
				return err
			default:
				if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindDocumentRenderPdf, api.DocumentRenderPdfJob{
					DocumentId: id, DownloadUrl: s.files.SignedURL(key, downloadLinkTTL),
				}); err != nil {
					return err
				}
				if err := audit.Record(ctx, tx, audit.Entry{Action: "document.pdf_requested", EntityType: "document", EntityID: id}); err != nil {
					return err
				}
			}
			if row, err = q.GetDocument(ctx, id); err != nil {
				return err
			}
		}
		out = fromRow(row)
		return nil
	})
	return out, err
}

// MakeDocumentPdf handles POST /v1/documents/{documentId}/pdf.
func (h *HTTP) MakeDocumentPdf(ctx context.Context, req api.MakeDocumentPdfRequestObject) (api.MakeDocumentPdfResponseObject, error) {
	doc, err := h.svc.MakePdf(ctx, authz.MustFromContext(ctx), req.DocumentId)
	if err != nil {
		return nil, err
	}
	return api.MakeDocumentPdf202JSONResponse(documentToAPI(doc, false)), nil
}
