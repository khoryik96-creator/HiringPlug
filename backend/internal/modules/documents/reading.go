package documents

// When reading a document ends without a result. A job of the AI worker that
// fails for good (its last attempt, or given up: a file gone) used to leave
// the CV "Reading details..." forever. Its dead-job handler now ends the
// reading: back to the details read before, with a warning, or failed with
// a reason people can act on. The worker's upkeep does the same for
// documents left being read with no job to finish them (a crash, a restore,
// before this). And a reading asked for again while one is under way is not
// queued twice.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// readingKinds are the jobs of a CV's reading (its text, then its details).
var readingKinds = []api.JobKind{
	api.JobKindDocumentExtractText, api.JobKindDocumentTextExtracted,
	api.JobKindDocumentParseCv, api.JobKindDocumentCvParsed,
}

// What people are told (the column allows 300 characters for parse_error).
const (
	keptWarning   = "The CV could not be read again, so the details read before are kept. Read it again later."
	noDetails     = "The details could not be read from this CV. Read it again, or fill them in by hand."
	textStopped   = "The CV's text could not be read: the reader stopped after several tries. Read it again; if it fails again, upload the file again."
	fileMissing   = "The file was not found on the server, so its text could not be read. Upload it again."
	readingLost   = "Reading this file stopped before it finished (the server was restarted or restored). Read it again."
	pdfNotMade    = "The PDF copy could not be made."
	missingReason = "HTTP 404"
	goneReason    = "HTTP 410"
)

// DeadJobHandlers end a document's reading, or its PDF copy, when the AI
// worker's job for it fails for good.
func (m *Module) DeadJobHandlers() jobqueue.Handlers {
	return jobqueue.Handlers{
		api.JobKindDocumentExtractText: m.service.extractionDied,
		api.JobKindDocumentParseCv:     m.service.parseDied,
		api.JobKindDocumentRenderPdf:   m.service.pdfDied,
	}
}

// extractionDied: the text could not be taken out of the file.
func (s *Service) extractionDied(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var p api.DocumentExtractTextJob
	if err := job.Decode(&p); err != nil {
		return err
	}
	why := textStopped
	if strings.Contains(job.LastError, missingReason) || strings.Contains(job.LastError, goneReason) {
		why = fileMissing
	}
	return s.endReading(ctx, tx, job.TenantID, p.DocumentId, why, "document.reading_failed", job.Kind)
}

// parseDied: the details could not be read from the text.
func (s *Service) parseDied(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var p api.DocumentParseCvJob
	if err := job.Decode(&p); err != nil {
		return err
	}
	return s.endReading(ctx, tx, job.TenantID, p.DocumentId, textStopped, "document.reading_failed", job.Kind)
}

// pdfDied: no PDF copy (the Word file itself is fine).
func (s *Service) pdfDied(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var p api.DocumentRenderPdfJob
	if err := job.Decode(&p); err != nil {
		return err
	}
	return s.failPdf(ctx, tx, p.DocumentId, string(job.Kind))
}

// failPdf ends a PDF copy that has no result coming, unless a newer one is
// under way (it decides).
func (s *Service) failPdf(ctx context.Context, tx pgx.Tx, id, why string) error {
	pdfKinds := []api.JobKind{api.JobKindDocumentRenderPdf, api.JobKindDocumentPdfRendered}
	if active, err := jobqueue.Active(ctx, tx, pdfKinds, "documentId", id); err != nil || active {
		return err
	}
	failed, msg := string(api.PdfStatusFailed), pdfNotMade
	_, err := store.New(tx).SavePdf(ctx, store.SavePdfParams{ID: id, PdfStatus: &failed, PdfError: &msg})
	if database.IsNoRows(err) {
		return nil // gone, or no longer waited for
	}
	if err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{Action: "document.pdf_failed", EntityType: "document", EntityID: id,
		Changes: map[string]any{"why": why}})
}

// endReading ends a document's reading that has no result coming: its text
// failed when it was never read, and its details go back to those read
// before (with a warning) or fail. Nothing changes while another reading of
// it is under way (asked for since): that one decides.
func (s *Service) endReading(ctx context.Context, tx pgx.Tx, tenantID, id, textWhy, action string, kind api.JobKind) error {
	if active, err := jobqueue.Active(ctx, tx, readingKinds, "documentId", id); err != nil || active {
		return err
	}
	q := store.New(tx)
	row, err := q.GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	changes := map[string]any{"job": kind}
	if row.Status == string(api.DocumentStatusQueued) {
		if row, err = q.FailExtraction(ctx, store.FailExtractionParams{ID: id, ErrorMessage: &textWhy}); err != nil {
			return err
		}
		changes["status"] = row.Status
	}
	if row.ParseStatus != nil && *row.ParseStatus == string(api.DocumentParseStatusQueued) {
		if row, err = q.EndParse(ctx, store.EndParseParams{ID: id, FailedError: noDetails, KeptWarning: keptWarning}); err != nil {
			return err
		}
		changes["parseStatus"] = *row.ParseStatus
	}
	if len(changes) == 1 {
		return nil // it had ended already
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "document", EntityID: id, Changes: changes}); err != nil {
		return err
	}
	return s.readDone(ctx, tx, tenantID, id, nil)
}

// StuckAfter is how long a document may show as being read with no job for
// it before the upkeep ends its reading.
const StuckAfter = 15 * time.Minute

// ReconcileReading ends the reading of documents left "being read" with no
// job to finish it, in every company (the worker's upkeep, at most limit at
// a time). It returns how many it set right.
func (m *Module) ReconcileReading(ctx context.Context, limit int) (int, error) {
	return reconcileReading(ctx, m.service, m.service.pool, limit)
}

func reconcileReading(ctx context.Context, s *Service, pool *pgxpool.Pool, limit int) (int, error) {
	rows, err := pool.Query(ctx, `SELECT tenant_id, document_id FROM document_reading_stuck(make_interval(secs => $1), $2)`,
		StuckAfter.Seconds(), limit)
	if err != nil {
		return 0, fmt.Errorf("finding documents left being read: %w", err)
	}
	type stuck struct{ tenant, id string }
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (stuck, error) {
		var st stuck
		err := r.Scan(&st.tenant, &st.id)
		return st, err
	})
	if err != nil {
		return 0, err
	}
	fixed := 0
	for _, st := range found {
		err := database.InTx(ctx, pool, authz.System(st.tenant), func(tx pgx.Tx) error {
			if err := s.endReading(ctx, tx, st.tenant, st.id, readingLost, "document.reading_reconciled", "upkeep"); err != nil {
				return err
			}
			row, err := store.New(tx).GetDocument(ctx, st.id)
			if database.IsNoRows(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if row.PdfStatus != nil && *row.PdfStatus == string(api.PdfStatusQueued) {
				return s.failPdf(ctx, tx, st.id, "upkeep")
			}
			return nil
		})
		if err != nil {
			logx.From(ctx).Warn("a document left being read could not be set right", "document_id", st.id, "error", err)
			continue
		}
		fixed++
	}
	return fixed, nil
}
