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
	linkExpired   = "The file's link expired before the reader could fetch it (the file itself is fine). Read it again."
	readingLost   = "Reading this file stopped before it finished (the server was restarted or restored). Read it again."
	pdfNotMade    = "The PDF copy could not be made."
	// What the AI worker's download says (ai/.../download.py): the file
	// server's own "not found", a refused link, and (before 1.47.1) any 404.
	missingReason = "file missing"
	expiredReason = "link expired"
	foreignReason = "other than HiringPlug's file server"
	legacyMissing = "HTTP 404"
	legacyGone    = "HTTP 410"
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
	switch e := job.LastError; {
	case strings.Contains(e, missingReason):
		why = fileMissing
	case strings.Contains(e, expiredReason):
		why = linkExpired
	case strings.Contains(e, foreignReason): // a setting, tried again: stopped after several tries
	case strings.Contains(e, legacyMissing), strings.Contains(e, legacyGone):
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
// it is under way (asked for since): that one decides. The document is
// locked first, so a reading asked for at the same moment is either seen
// (its job) or waits for this to end; one already ended is left as it is.
func (s *Service) endReading(ctx context.Context, tx pgx.Tx, tenantID, id, textWhy, action string, kind api.JobKind) error {
	q := store.New(tx)
	if _, err := q.LockDocument(ctx, id); database.IsNoRows(err) {
		return nil
	} else if err != nil {
		return err
	}
	if active, err := jobqueue.Active(ctx, tx, readingKinds, "documentId", id); err != nil || active {
		return err
	}
	row, err := q.GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	changes := map[string]any{"job": kind}
	if row.Status == string(api.DocumentStatusQueued) {
		switch ended, err := q.FailExtraction(ctx, store.FailExtractionParams{ID: id, ErrorMessage: &textWhy}); {
		case database.IsNoRows(err): // ended already
		case err != nil:
			return err
		default:
			row, changes["status"] = ended, ended.Status
		}
	}
	if row.ParseStatus != nil && *row.ParseStatus == string(api.DocumentParseStatusQueued) {
		switch ended, err := q.EndParse(ctx, store.EndParseParams{ID: id, FailedError: noDetails, KeptWarning: keptWarning}); {
		case database.IsNoRows(err): // ended already
		case err != nil:
			return err
		default:
			row, changes["parseStatus"] = ended, *ended.ParseStatus
		}
	}
	if len(changes) == 1 {
		return nil // it had ended already
	}
	if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "document", EntityID: id, Changes: changes}); err != nil {
		return err
	}
	return s.readDone(ctx, tx, tenantID, id, nil)
}

// readingJobs works out, for the documents being read, how far the job
// reading them is (none left: the reading stopped, and reading again is
// offered), so a page can tell a long wait from a reading that stopped.
func readingJobs(ctx context.Context, tx pgx.Tx, docs []Document) error {
	var reading []string
	for _, d := range docs {
		if beingRead(d) {
			reading = append(reading, d.ID)
		}
	}
	jobs, err := jobqueue.ActiveFor(ctx, tx, readingKinds, "documentId", reading)
	if err != nil {
		return err
	}
	for i, d := range docs {
		if beingRead(d) {
			docs[i].ReadingKnown = true
			if state, ok := jobs[d.ID]; ok {
				docs[i].ReadingJob = &state
			}
		}
	}
	return nil
}

func beingRead(d Document) bool {
	return d.Status == string(api.DocumentStatusQueued) || (d.ParseStatus != nil && *d.ParseStatus == string(api.DocumentParseStatusQueued))
}

// StuckAfter is how long a document may show as being read with no job for
// it before the upkeep ends its reading.
const StuckAfter = 15 * time.Minute

// ReconcileReading ends the reading of documents left "being read" with no
// job to finish it, in every company (the worker's upkeep, at most limit at
// a time), oldest first. One it could not set right is left out for a day,
// so a few that keep failing never crowd the others out. It returns how
// many it set right.
func (m *Module) ReconcileReading(ctx context.Context, limit int) (int, error) {
	return m.service.reconcileReading(ctx, limit, time.Now())
}

// reconcileAgainAfter is how long the upkeep leaves a document it could not set right.
const reconcileAgainAfter = 24 * time.Hour

func (s *Service) reconcileReading(ctx context.Context, limit int, now time.Time) (int, error) {
	s.stuckMu.Lock()
	skip := []string{}
	for id, at := range s.stuckFailed {
		if now.Sub(at) < reconcileAgainAfter {
			skip = append(skip, id)
		} else {
			delete(s.stuckFailed, id)
		}
	}
	s.stuckMu.Unlock()
	found, err := store.New(s.pool).ReadingStuck(ctx, store.ReadingStuckParams{
		OlderSeconds: StuckAfter.Seconds(), MaxCount: int32(min(limit, 10000)), Skip: skip, //nolint:gosec // capped
	})
	if err != nil {
		return 0, fmt.Errorf("finding documents left being read: %w", err)
	}
	fixed := 0
	for _, st := range found {
		err := database.InTx(ctx, s.pool, authz.System(st.TenantID), func(tx pgx.Tx) error {
			if err := s.endReading(ctx, tx, st.TenantID, st.DocumentID, readingLost, "document.reading_reconciled", "upkeep"); err != nil {
				return err
			}
			row, err := store.New(tx).GetDocument(ctx, st.DocumentID)
			if database.IsNoRows(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if row.PdfStatus != nil && *row.PdfStatus == string(api.PdfStatusQueued) {
				return s.failPdf(ctx, tx, st.DocumentID, "upkeep")
			}
			return nil
		})
		if err != nil {
			logx.From(ctx).Warn("a document left being read could not be set right", "document_id", st.DocumentID, "error", err)
			s.stuckMu.Lock()
			s.stuckFailed[st.DocumentID] = now
			s.stuckMu.Unlock()
			continue
		}
		fixed++
	}
	return fixed, nil
}
