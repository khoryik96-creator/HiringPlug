package documents

import (
	"context"

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
	_, err := store.New(tx).SaveExtraction(ctx, store.SaveExtractionParams{
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
	return audit.Record(ctx, tx, audit.Entry{
		Action: "document.text_extracted", EntityType: "document", EntityID: result.DocumentId,
		Changes: map[string]any{"status": result.Status, "extractor": result.Extractor, "warnings": len(result.Warnings)},
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
