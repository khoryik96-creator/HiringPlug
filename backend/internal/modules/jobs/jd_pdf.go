package jobs

// A Word (or RTF, ODT) job description also comes as a PDF, so Job details
// and the job's ads preview it as it looks: the Python worker turns the file
// into one with LibreOffice, and the Go worker saves it with the file.

import (
	"context"
	"errors"
	"io"
	"mime"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/jobs/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/filekind"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/pdfcopy"
)

// queueJDPdf asks the Python worker for a job description's PDF copy.
func (s *Service) queueJDPdf(ctx context.Context, tx pgx.Tx, tenantID, id, key string) error {
	if err := store.New(tx).QueueJdPdf(ctx, id); err != nil {
		return err
	}
	_, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindJdRenderPdf, api.JdRenderPdfJob{
		JdFileId: id, DownloadUrl: s.files.SignedURL(key, jdLinkTTL),
	})
	return err
}

// saveJDPdf handles jd.pdf_rendered: the PDF, or why it failed.
func (s *Service) saveJDPdf(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.JdPdfRenderedJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	saved, err := pdfcopy.Save(s.files, job.TenantID, result.JdFileId, job.ID, result.PdfBase64, result.Error)
	if err != nil {
		return err
	}
	_, err = store.New(tx).SaveJdPdf(ctx, store.SaveJdPdfParams{ID: result.JdFileId, PdfStatus: &saved.Status, PdfKey: saved.Key, PdfError: saved.Error})
	if database.IsNoRows(err) {
		// Gone, or not waited for: only this result's own file goes.
		logx.From(ctx).Info("job description gone or its PDF not waited for; dropping it", "jd_file_id", result.JdFileId)
		if saved.Key != nil {
			_ = s.files.Delete(*saved.Key)
		}
		return nil
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "job.jd_pdf_rendered", EntityType: "jd_file", EntityID: result.JdFileId, Changes: map[string]any{"status": saved.Status}})
}

// MakeJDPdf queues a Word job description's PDF copy when it has none (one
// uploaded before previews began); anything else is returned as it is.
func (s *Service) MakeJDPdf(ctx context.Context, p authz.Principal, id string) (JDFile, error) {
	var out JDFile
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetJdFile(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("job description")
		}
		if err != nil {
			return err
		}
		if filekind.Office(row.ContentType) && row.PdfStatus == nil { // a failed copy is not tried again on every view
			key, err := q.QueueJdPdfIfNone(ctx, id) // two asking at once: one queues it
			switch {
			case database.IsNoRows(err):
			case err != nil:
				return err
			default:
				if _, err := jobqueue.Enqueue(ctx, tx, p.TenantID, api.JobKindJdRenderPdf, api.JdRenderPdfJob{
					JdFileId: id, DownloadUrl: s.files.SignedURL(key, jdLinkTTL),
				}); err != nil {
					return err
				}
				if err := audit.Record(ctx, tx, audit.Entry{Action: "job.jd_pdf_requested", EntityType: "jd_file", EntityID: id}); err != nil {
					return err
				}
			}
			if row, err = q.GetJdFile(ctx, id); err != nil {
				return err
			}
		}
		out, err = jdFrom(row)
		return err
	})
	return out, err
}

// JDPdf opens a job description's PDF copy.
func (s *Service) JDPdf(ctx context.Context, p authz.Principal, id string) (string, io.ReadCloser, error) {
	var row store.GetJdFileStorageRow
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		var err error
		row, err = store.New(tx).GetJdFileStorage(ctx, id)
		if database.IsNoRows(err) {
			return apperr.NotFound("job description")
		}
		return err
	})
	if err != nil {
		return "", nil, err
	}
	if row.PdfKey == nil {
		return "", nil, apperr.NotFound("PDF of this job description")
	}
	f, err := s.files.Open(*row.PdfKey)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, apperr.NotFound("PDF of this job description")
	}
	return pdfcopy.FileName(row.FileName), f, err
}

// GetJobDescriptionPdf handles GET /v1/jd-files/{jdFileId}/pdf.
func (h *HTTP) GetJobDescriptionPdf(ctx context.Context, req api.GetJobDescriptionPdfRequestObject) (api.GetJobDescriptionPdfResponseObject, error) {
	name, f, err := h.svc.JDPdf(ctx, authz.MustFromContext(ctx), req.JdFileId)
	if err != nil {
		return nil, err
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": name})
	return api.GetJobDescriptionPdf200ApplicationoctetStreamResponse{
		Body: f, Headers: api.GetJobDescriptionPdf200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

// MakeJobDescriptionPdf handles POST /v1/jd-files/{jdFileId}/pdf.
func (h *HTTP) MakeJobDescriptionPdf(ctx context.Context, req api.MakeJobDescriptionPdfRequestObject) (api.MakeJobDescriptionPdfResponseObject, error) {
	f, err := h.svc.MakeJDPdf(ctx, authz.MustFromContext(ctx), req.JdFileId)
	if err != nil {
		return nil, err
	}
	return api.MakeJobDescriptionPdf202JSONResponse(jdToAPI(f)), nil
}
