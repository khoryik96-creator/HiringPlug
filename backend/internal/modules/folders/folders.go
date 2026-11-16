// Package folders keeps folders of candidates, contacts or companies: each
// person makes their own and may share one with others of the same agency,
// who can then add and take out records; only the owner renames, deletes or
// shares it. The lists (candidates, contacts, companies) show a folder's
// records through Items.
//
// Layout (the same in every module):
//
//	folders.go     the module's public API: what internal/app and other modules may use
//	domain.go     the rules, as pure functions on plain values (unit tested)
//	service.go    use cases: one transaction each, rules + storage + audit
//	http.go       HTTP handlers: contract types <-> service calls; no business rules
//	internal/store  generated SQL (db/queries/folders.sql); private to this module
package folders

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/folders/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
)

// Module is the folders module.
type Module struct {
	service *Service
	HTTP    *HTTP
}

// New wires the module.
func New(pool *pgxpool.Pool, users Users, records Records) *Module {
	svc := &Service{pool: pool, users: users, records: records}
	return &Module{service: svc, HTTP: &HTTP{svc: svc}}
}

// Items returns the ids in a folder of kind that the signed-in person may see
// (owner or shared), newest first, inside the caller's transaction: a list
// shows only those. A folder they may not see, or of another kind, is not found.
func (m *Module) Items(ctx context.Context, tx pgx.Tx, p authz.Principal, folderID, kind string) ([]string, error) {
	q := store.New(tx)
	got, err := q.VisibleFolderKind(ctx, folderID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && got != kind) {
		return nil, apperr.NotFound("folder")
	}
	if err != nil {
		return nil, err
	}
	ids, err := q.FolderItemIDs(ctx, folderID)
	if ids == nil {
		ids = []string{}
	}
	return ids, err
}

// ItemsOf is Items for one kind of record, for the list of that kind.
func (m *Module) ItemsOf(kind string) func(ctx context.Context, tx pgx.Tx, p authz.Principal, folderID string) ([]string, error) {
	return func(ctx context.Context, tx pgx.Tx, p authz.Principal, folderID string) ([]string, error) {
		return m.Items(ctx, tx, p, folderID, kind)
	}
}

// MoveCandidate puts a merged duplicate's place in folders on the record kept
// (once in each folder), in the merge's transaction.
func (m *Module) MoveCandidate(ctx context.Context, tx pgx.Tx, fromID, toID string) error {
	q := store.New(tx)
	if err := q.MoveItems(ctx, store.MoveItemsParams{FromID: fromID, ToID: toID}); err != nil {
		return err
	}
	return q.DropItem(ctx, fromID) // the folders that held both
}
