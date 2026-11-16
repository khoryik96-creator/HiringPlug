package app

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/candidates"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/companies"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/contacts"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/folders"
	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/identity"
)

// folderUsers: a folder is shared only with active users of the same agency.
type folderUsers struct{ identity *identity.Module }

func (u folderUsers) Active(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	_, ok, err := u.identity.PrincipalFor(ctx, tx, userID)
	return ok, err
}

// folderRecords: only records that exist and the person may see go in a
// folder (each module's queries run under its row-level security).
type folderRecords struct {
	candidates *candidates.Module
	contacts   *contacts.Module
	companies  *companies.Module
}

func (r folderRecords) Visible(ctx context.Context, tx pgx.Tx, kind string, ids []string) ([]string, error) {
	var found func(id string) bool
	var err error
	switch kind {
	case folders.Candidates:
		found, err = has(r.candidates.Briefs(ctx, tx, ids))
	case folders.Contacts:
		found, err = has(r.contacts.Briefs(ctx, tx, ids))
	default:
		found, err = has(r.companies.Names(ctx, tx, ids))
	}
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return !found(id) }), nil
}

// has turns what a module found (by id) into a lookup.
func has[V any](got map[string]V, err error) (func(id string) bool, error) {
	return func(id string) bool { _, ok := got[id]; return ok }, err
}
