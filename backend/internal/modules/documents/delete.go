package documents

// Deleting a formatted CV: one HiringPlug made can go (with its stored file
// and PDF copy); the candidate's own files are always kept.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// DeleteFormatted deletes a formatted CV. Its files are removed once the
// deletion is committed (a file that cannot be removed is logged, not an
// error: nothing refers to it any more).
func (s *Service) DeleteFormatted(ctx context.Context, p authz.Principal, id string) error {
	var keys []string
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		q := store.New(tx)
		row, err := q.GetDocument(ctx, id) // row-level security: only documents the user may see
		if database.IsNoRows(err) {
			return apperr.NotFound("document")
		}
		if err != nil {
			return err
		}
		if row.Source != "formatted" {
			return apperr.New(api.ErrorCodeDOCUMENTNOTDELETABLE,
				"Only formatted CVs can be deleted; the candidate's own CVs are kept.")
		}
		gone, err := q.DeleteFormattedDocument(ctx, id)
		if err != nil {
			return err
		}
		keys = append(keys, gone.StorageKey)
		if gone.PdfKey != nil {
			keys = append(keys, *gone.PdfKey)
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "document.deleted", EntityType: "document", EntityID: id,
			Changes: map[string]any{"candidateId": row.CandidateID, "fileName": row.FileName, "source": row.Source}})
	})
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.files.Delete(k); err != nil {
			logx.From(ctx).Warn("a deleted formatted CV's file could not be removed", "document_id", id, "error", err)
		}
	}
	return nil
}

// DeleteDocument handles DELETE /v1/documents/{documentId}.
func (h *HTTP) DeleteDocument(ctx context.Context, req api.DeleteDocumentRequestObject) (api.DeleteDocumentResponseObject, error) {
	if err := h.svc.DeleteFormatted(ctx, authz.MustFromContext(ctx), req.DocumentId); err != nil {
		return nil, err
	}
	return api.DeleteDocument204Response{}, nil
}
