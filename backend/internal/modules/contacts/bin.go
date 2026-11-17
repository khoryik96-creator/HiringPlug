package contacts

// The Bin (bin module): binned contacts are hidden by the database until
// restored, then deleted for good. Called inside the bin module's
// transaction, which sees binned records.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// ErrEmailTaken: a contact in the Bin cannot be restored while another has its email.
var ErrEmailTaken = errors.New("another contact has this email now")

// Label is a contact's name (not found when binned).
func (m *Module) Label(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	label, err := store.New(tx).ContactLabel(ctx, id)
	if database.IsNoRows(err) {
		return "", apperr.NotFound("contact")
	}
	return label, err
}

// Bin hides contacts and returns their names by id (those already binned are left out).
func (m *Module) Bin(ctx context.Context, tx pgx.Tx, ids []string) (map[string]string, error) {
	rows, err := store.New(tx).BinContacts(ctx, ids)
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Label
	}
	return out, err
}

// Restore shows binned contacts again.
func (m *Module) Restore(ctx context.Context, tx pgx.Tx, ids []string) error {
	_, err := store.New(tx).RestoreContacts(ctx, ids)
	if database.IsUniqueViolation(err, emailUniqueIndex) {
		return ErrEmailTaken
	}
	return err
}

// IDsOfCompany returns a company's contacts (not those in the Bin).
func (m *Module) IDsOfCompany(ctx context.Context, tx pgx.Tx, companyID string) ([]string, error) {
	return store.New(tx).ContactIDsOfCompany(ctx, companyID)
}

// MoveToCompany moves a company's contacts to another company.
func (m *Module) MoveToCompany(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	_, err := store.New(tx).MoveContactsToCompany(ctx, store.MoveContactsToCompanyParams{FromID: fromID, ToID: toID})
	return err
}

// Purge deletes binned contacts for good (their checks go with them).
func (m *Module) Purge(ctx context.Context, tx pgx.Tx, ids []string) error {
	_, err := store.New(tx).PurgeContacts(ctx, ids)
	return err
}
