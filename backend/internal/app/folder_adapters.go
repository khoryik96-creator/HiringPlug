package app

import (
	"context"

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
	switch kind {
	case folders.Candidates:
		got, err := r.candidates.Briefs(ctx, tx, ids)
		if err != nil {
			return nil, err
		}
		found = func(id string) bool { _, ok := got[id]; return ok }
	case folders.Contacts:
		got, err := r.contacts.Briefs(ctx, tx, ids)
		if err != nil {
			return nil, err
		}
		found = func(id string) bool { _, ok := got[id]; return ok }
	default:
		got, err := r.companies.Names(ctx, tx, ids)
		if err != nil {
			return nil, err
		}
		found = func(id string) bool { _, ok := got[id]; return ok }
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if found(id) {
			out = append(out, id)
		}
	}
	return out, nil
}
