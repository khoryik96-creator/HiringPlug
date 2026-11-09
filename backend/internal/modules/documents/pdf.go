package documents

// Word documents also come as a PDF (formatted CVs to send, CVs sent as Word
// to preview): the Python worker turns the Word file into one with
// LibreOffice, and the Go worker saves it with the document.

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apiconv"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
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
	status, key := string(api.PdfStatusFailed), (*string)(nil)
	errMsg := apiconv.ValueOrNil(result.Error)
	if encoded, err := result.PdfBase64.Get(); err == nil {
		pdf, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || !strings.HasPrefix(string(pdf), "%PDF-") {
			msg := "the PDF could not be read"
			errMsg = &msg
		} else {
			k := job.TenantID + "/" + result.DocumentId + "_pdf"
			if err := s.files.Put(k, pdf); err != nil {
				return err
			}
			status, key, errMsg = string(api.PdfStatusReady), &k, nil
		}
	}
	_, err := store.New(tx).SavePdf(ctx, store.SavePdfParams{ID: result.DocumentId, PdfStatus: &status, PdfKey: key, PdfError: errMsg})
	if database.IsNoRows(err) {
		logx.From(ctx).Info("document gone or its PDF not waited for; dropping it", "document_id", result.DocumentId)
		if key != nil {
			_ = s.files.Delete(*key)
		}
		return nil
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "document.pdf_rendered", EntityType: "document", EntityID: result.DocumentId, Changes: map[string]any{"status": status}})
}

// Pdf opens a document's PDF.
func (s *Service) Pdf(ctx context.Context, p authz.Principal, id string) (Document, io.ReadCloser, error) {
	var row store.Document
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).GetDocument(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		return err
	})
	if err != nil {
		return Document{}, nil, err
	}
	if row.PdfKey == nil {
		return Document{}, nil, apperr.NotFound("PDF of this document")
	}
	f, err := s.files.Open(*row.PdfKey)
	if errors.Is(err, os.ErrNotExist) {
		return Document{}, nil, apperr.NotFound("PDF of this document")
	}
	return fromRow(row), f, err
}

// GetDocumentPdf handles GET /v1/documents/{documentId}/pdf.
func (h *HTTP) GetDocumentPdf(ctx context.Context, req api.GetDocumentPdfRequestObject) (api.GetDocumentPdfResponseObject, error) {
	doc, f, err := h.svc.Pdf(ctx, authz.MustFromContext(ctx), req.DocumentId)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(strings.TrimSuffix(doc.FileName, ".docx"), ".doc") + ".pdf"
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
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
		if filekind.Office(row.ContentType) && row.PdfStatus == nil { // a failed copy is not tried again on every view
			if err := s.queuePdf(ctx, tx, p.TenantID, fromRow(row), row.StorageKey); err != nil {
				return err
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
