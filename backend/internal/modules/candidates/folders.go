package candidates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Folders gives the candidates in a folder the signed-in person may see (the
// folders module): a list in a folder shows only those.
type Folders interface {
	Items(ctx context.Context, tx pgx.Tx, p authz.Principal, folderID, kind string) ([]string, error)
}

// UseFolders sets where folders come from (internal/app).
func (m *Module) UseFolders(f Folders) { m.service.folders = f }

// folderIDs is nil without a folder (every candidate), else the folder's candidates.
func (s *Service) folderIDs(ctx context.Context, tx pgx.Tx, p authz.Principal, folder *string) ([]string, error) {
	if folder == nil {
		return nil, nil
	}
	if s.folders == nil {
		return nil, apperr.NotFound("folder")
	}
	return s.folders.Items(ctx, tx, p, *folder, "candidate")
}
