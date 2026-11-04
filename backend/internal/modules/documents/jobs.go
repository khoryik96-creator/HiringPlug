package documents

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/jobqueue"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// saveExtraction handles document.text_extracted: the Python worker's result.
// It runs in a transaction scoped to the job's tenant.
func (s *Service) saveExtraction(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.DocumentTextExtractedJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	var text *string
	if t, err := result.Text.Get(); err == nil {
		cleaned := CleanExtractedText(t)
		text = &cleaned
	}
	var pages *int32
	if n, err := result.PageCount.Get(); err == nil {
		v := int32(n) //nolint:gosec // page counts are small
		pages = &v
	}
	var errMsg *string
	if m, err := result.ErrorMessage.Get(); err == nil {
		errMsg = &m
	}
	row, err := store.New(tx).SaveExtraction(ctx, store.SaveExtractionParams{
		ID: result.DocumentId, Status: string(result.Status), ExtractedText: text,
		PageCount: pages, Extractor: &result.Extractor, ErrorMessage: errMsg,
		ExtractionWarnings: nonNil(result.Warnings),
	})
	if database.IsNoRows(err) {
		logx.From(ctx).Info("document no longer exists; dropping its extraction result", "document_id", result.DocumentId)
		return nil
	}
	if err != nil {
		return err
	}
	// Every CV is parsed as soon as its text is saved: candidates' CVs for
	// review, formatted ones (our own output) for comparing with the original.
	if row.Status == string(api.DocumentStatusExtracted) && hasText(row.ExtractedText) {
		if err := queueParse(ctx, tx, job.TenantID, row); err != nil {
			return err
		}
		if row.Source != "formatted" {
			for _, hook := range s.whenRead {
				if err := hook(ctx, tx, job.TenantID, row.CandidateID); err != nil {
					return err
				}
			}
		}
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "document.text_extracted", EntityType: "document", EntityID: result.DocumentId,
		Changes: map[string]any{"status": result.Status, "extractor": result.Extractor, "warnings": len(result.Warnings)},
	})
}

// queueParse asks the AI worker to parse a document's saved text.
func queueParse(ctx context.Context, tx pgx.Tx, tenantID string, row store.Document) error {
	if _, err := jobqueue.Enqueue(ctx, tx, tenantID, api.JobKindDocumentParseCv, api.DocumentParseCvJob{
		DocumentId: row.ID, Text: *row.ExtractedText,
	}); err != nil {
		return err
	}
	_, err := store.New(tx).QueueParse(ctx, row.ID)
	return err
}

// saveParse handles document.cv_parsed: the parser's result. Only values
// whose quote is in the saved CV text are kept (groundProfile).
func (s *Service) saveParse(ctx context.Context, tx pgx.Tx, job jobqueue.Job) error {
	var result api.DocumentCvParsedJob
	if err := job.Decode(&result); err != nil {
		return err
	}
	q := store.New(tx)
	row, err := q.GetDocument(ctx, result.DocumentId)
	if database.IsNoRows(err) {
		logx.From(ctx).Info("document no longer exists; dropping its parse result", "document_id", result.DocumentId)
		return nil
	}
	if err != nil {
		return err
	}
	status := string(api.DocumentParseStatusFailed)
	params := store.SaveParseParams{ID: row.ID, ParseStatus: &status}
	changes := map[string]any{"status": status}
	if profile, err := result.Profile.Get(); err == nil && hasText(row.ExtractedText) {
		grounded := groundProfile(*row.ExtractedText, profile)
		data, err := json.Marshal(grounded)
		if err != nil {
			return err
		}
		status = string(api.DocumentParseStatusParsed)
		params.ParsedProfile, params.Parser = data, &grounded.Parser
		// Counts only: CV contents never go in the audit log.
		changes = map[string]any{
			"status": status, "parser": grounded.Parser, "jobs": len(grounded.Experience),
			"skills": len(grounded.Skills), "dropped": grounded.Dropped,
		}
	}
	if _, err := q.SaveParse(ctx, params); err != nil {
		return err
	}
	return audit.Record(ctx, tx, audit.Entry{
		Action: "document.cv_parsed", EntityType: "document", EntityID: row.ID, Changes: changes,
	})
}

func hasText(t *string) bool {
	return t != nil && strings.TrimSpace(*t) != ""
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
