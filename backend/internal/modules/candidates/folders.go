package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// FolderItems gives the candidates in a folder that the signed-in person may
// see (the folders module, through internal/app): a list in a folder shows
// only those. A folder they may not see is not found.
type FolderItems func(ctx context.Context, tx pgx.Tx, p authz.Principal, folderID string) ([]string, error)

// UseFolders sets where a folder's candidates come from (internal/app).
func (m *Module) UseFolders(f FolderItems) { m.service.folderItems = f }

// folderIDs is nil without a folder (every candidate), else the folder's candidates.
func (s *Service) folderIDs(ctx context.Context, tx pgx.Tx, p authz.Principal, folder *string) ([]string, error) {
	if folder == nil {
		return nil, nil
	}
	if s.folderItems == nil {
		return nil, apperr.NotFound("folder")
	}
	return s.folderItems(ctx, tx, p, *folder)
}
