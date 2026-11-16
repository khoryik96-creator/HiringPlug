// Package folderfilter is how a list (candidates, contacts, companies) shows
// only the records in a folder: the folders module gives each list a
// function for its kind (wired in internal/app), so the lists need not know
// the folders module.
package folderfilter

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Items gives the records in a folder that the signed-in person may see. A
// folder they may not see, or of another kind, is not found.
type Items func(ctx context.Context, tx pgx.Tx, p authz.Principal, folderID string) ([]string, error)

// IDs is nil without a folder (every record), else the folder's records.
func (f Items) IDs(ctx context.Context, tx pgx.Tx, p authz.Principal, folder *string) ([]string, error) {
	if folder == nil {
		return nil, nil
	}
	if f == nil {
		return nil, apperr.NotFound("folder")
	}
	return f(ctx, tx, p, *folder)
}
