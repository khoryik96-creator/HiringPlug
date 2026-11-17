package companies

// The Bin (bin module): a binned company is hidden by the database until
// restored, then deleted for good. Called inside the bin module's
// transaction, which sees binned records.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/logx"
)

// ErrNameTaken: a company in the Bin cannot be restored while another has its name.
var ErrNameTaken = errors.New("another company has this name now")

// Label is a company's name (not found when binned).
func (m *Module) Label(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	name, err := store.New(tx).CompanyLabel(ctx, id)
	if database.IsNoRows(err) {
		return "", apperr.NotFound("company")
	}
	return name, err
}

// Bin hides a company and returns its name.
func (m *Module) Bin(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	name, err := store.New(tx).BinCompany(ctx, id)
	if database.IsNoRows(err) {
		return "", apperr.NotFound("company")
	}
	return name, err
}

// Restore shows a binned company again.
func (m *Module) Restore(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := store.New(tx).RestoreCompany(ctx, id)
	if database.IsUniqueViolation(err, nameUniqueIndex) {
		return ErrNameTaken
	}
	return err
}

// Purge deletes a binned company for good (its terms, benefits and documents
// with it); after commits, its stored files are removed.
func (m *Module) Purge(ctx context.Context, tx pgx.Tx, id string) (after func(), err error) {
	keys, err := store.New(tx).PurgeCompany(ctx, id)
	return func() { removeFiles(ctx, m.service.files, keys) }, err
}

type remover interface{ Delete(key string) error }

func removeFiles(ctx context.Context, files remover, keys []string) {
	for _, k := range keys {
		if err := files.Delete(k); err != nil {
			logx.From(ctx).Warn("a deleted company's file could not be removed", "error", err)
		}
	}
}
