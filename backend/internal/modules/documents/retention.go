package documents

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/api"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/documents/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

const (
	formattedLimit = 3
	originalLimit  = 4
)

func cvLimit(formatted bool) int {
	if formatted {
		return formattedLimit
	}
	return originalLimit
}

func cvLimitError(need int, formatted bool) *apperr.Error {
	kind := "non-formatted"
	if formatted {
		kind = "formatted"
	}
	return apperr.New(api.ErrorCodeDOCUMENTLIMIT, fmt.Sprintf("Choose which %s CVs to replace. This candidate can keep %d.", kind, cvLimit(formatted))).
		WithDetail("required", need).WithDetail("limit", cvLimit(formatted))
}

// BinWith records replaced documents in the agency's Bin in the same transaction.
func (m *Module) BinWith(record func(ctx context.Context, tx pgx.Tx, tenantID, documentID, fileName, actorID string) error) {
	m.service.recordBinned = record
}

// CheckFormattedCapacity validates a selection before scheduling CV Studio.
// The same check runs again when its replacement is ready to be saved.
func (m *Module) CheckFormattedCapacity(ctx context.Context, tx pgx.Tx, candidateID string, replace []string) error {
	return checkCVSlots(ctx, tx, candidateID, true, replace)
}

func checkCVSlots(ctx context.Context, tx pgx.Tx, candidateID string, formatted bool, replace []string) error {
	q := store.New(tx)
	if err := q.LockCandidateCVs(ctx, candidateID); err != nil {
		return err
	}
	ids, err := q.CVsInCategory(ctx, store.CVsInCategoryParams{CandidateID: candidateID, Formatted: formatted})
	if err != nil {
		return err
	}
	required := max(0, len(ids)+1-cvLimit(formatted))
	if len(replace) != required {
		return cvLimitError(required, formatted)
	}
	for i, id := range replace {
		if !slices.Contains(ids, id) || slices.Contains(replace[:i], id) {
			return cvLimitError(required, formatted)
		}
	}
	return nil
}

func (s *Service) binReplaced(ctx context.Context, tx pgx.Tx, tenantID, actorID string, replace []string) error {
	if len(replace) == 0 {
		return nil
	}
	return withBinnedDocuments(ctx, tx, func() error {
		q := store.New(tx)
		for _, id := range replace {
			row, err := q.GetDocument(ctx, id)
			if err != nil {
				return err
			}
			n, err := q.BinDocument(ctx, id)
			if err != nil {
				return err
			}
			if n != 1 {
				return apperr.NotFound("active document")
			}
			if err := s.recordBinned(ctx, tx, tenantID, row.ID, row.FileName, actorID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DocumentLabel resolves an active CV through its visible candidate.
func (m *Module) DocumentLabel(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	row, err := store.New(tx).GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return "", apperr.NotFound("document")
	}
	if err != nil {
		return "", err
	}
	active, err := m.service.candidates.Exists(ctx, tx, row.CandidateID)
	if err == nil && (!active || row.BinnedAt != nil) {
		return "", apperr.NotFound("active document")
	}
	return row.FileName, err
}

// BinActiveDocument frees a slot using the same lock as upload and restore.
func (m *Module) BinActiveDocument(ctx context.Context, tx pgx.Tx, id string) error {
	q := store.New(tx)
	row, err := q.GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return apperr.NotFound("document")
	}
	if err != nil {
		return err
	}
	if err := q.LockCandidateCVs(ctx, row.CandidateID); err != nil {
		return err
	}
	n, err := q.BinDocument(ctx, id)
	if err == nil && n != 1 {
		return apperr.NotFound("active document")
	}
	return err
}

// RestoreDocument restores one file from the Bin when its category has room.
func (m *Module) RestoreDocument(ctx context.Context, tx pgx.Tx, id string) error {
	q := store.New(tx)
	row, err := q.GetDocument(ctx, id)
	if database.IsNoRows(err) {
		return apperr.NotFound("document")
	}
	if err != nil {
		return err
	}
	binned, err := m.service.candidates.Binned(ctx, tx, []string{row.CandidateID})
	if err != nil {
		return err
	}
	if len(binned) != 0 {
		return apperr.New(api.ErrorCodeBINRESTORECONFLICT, "Restore the candidate before restoring their CV.")
	}
	if err := checkCVSlots(ctx, tx, row.CandidateID, row.Source == "formatted", nil); err != nil {
		if e, ok := apperr.As(err); ok && e.Code == api.ErrorCodeDOCUMENTLIMIT {
			return apperr.New(api.ErrorCodeBINRESTORECONFLICT, "This candidate's CV category is full. Free a slot before restoring this CV.")
		}
		return err
	}
	n, err := q.RestoreBinnedDocument(ctx, id)
	if err == nil && n != 1 {
		return apperr.NotFound("binned document")
	}
	return err
}

// PurgeDocument removes a binned document and returns the files to delete after commit.
func (m *Module) PurgeDocument(ctx context.Context, tx pgx.Tx, id string) ([]string, error) {
	row, err := store.New(tx).PurgeBinnedDocument(ctx, id)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	keys := []string{row.StorageKey}
	if row.PdfKey != nil {
		keys = append(keys, *row.PdfKey)
	}
	return keys, nil
}

// withBinnedDocuments lets the caller update documents that are entering or
// already in the Bin, without widening visibility for the rest of the transaction.
func withBinnedDocuments(ctx context.Context, tx pgx.Tx, run func() error) error {
	var previous string
	if err := tx.QueryRow(ctx, "SELECT coalesce(current_setting('app.show_binned', true), '')").Scan(&previous); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.show_binned', 'on', true)"); err != nil {
		return err
	}
	if err := run(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "SELECT set_config('app.show_binned', $1, true)", previous)
	return err
}
