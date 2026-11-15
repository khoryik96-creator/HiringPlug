package folders

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khoryik96-creator/hiringplug/backend/internal/modules/folders/internal/store"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/apperr"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/audit"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/authz"
	"github.com/khoryik96-creator/hiringplug/backend/internal/platform/database"
)

// Users says whether someone is an active user of the signed-in agency (the
// identity module): a folder is shared only with them.
type Users interface {
	Active(ctx context.Context, tx pgx.Tx, userID string) (bool, error)
}

// Records says which of some candidates, contacts or companies (kind) exist
// and the signed-in person may see (their modules): only those go in a folder.
type Records interface {
	Visible(ctx context.Context, tx pgx.Tx, kind string, ids []string) ([]string, error)
}

// Service holds the folders use cases.
type Service struct {
	pool    *pgxpool.Pool
	users   Users
	records Records
}

// held is load for a change to the folder's records or people: the folder is
// held (FOR SHARE) until the change is done, so it cannot be deleted halfway.
func held(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Folder, error) {
	if _, err := store.New(tx).LockFolder(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, apperr.NotFound("folder")
	} else if err != nil {
		return Folder{}, err
	}
	return load(ctx, tx, p, id)
}

// load returns a folder the signed-in person may see (owner or shared).
func load(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Folder, error) {
	row, err := store.New(tx).VisibleFolder(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Folder{}, apperr.NotFound("folder")
	}
	if err != nil {
		return Folder{}, err
	}
	return fromRow(store.VisibleFoldersRow(row), p), nil
}

// owned is load, for what only the owner may do.
func owned(ctx context.Context, tx pgx.Tx, p authz.Principal, id string) (Folder, error) {
	f, err := load(ctx, tx, p, id)
	if err == nil && !f.Mine {
		err = OwnerOnly()
	}
	return f, err
}

// List returns the folders of kind the signed-in person owns or that are
// shared with them, by name.
func (s *Service) List(ctx context.Context, p authz.Principal, kind string) ([]Folder, error) {
	var out []Folder
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).VisibleFolders(ctx, kind)
		if err != nil {
			return err
		}
		out = make([]Folder, len(rows))
		for i, r := range rows {
			out[i] = fromRow(r, p)
		}
		return nil
	})
	return out, err
}

// Create makes a folder, the signed-in person's own.
func (s *Service) Create(ctx context.Context, p authz.Principal, kind, name string) (Folder, error) {
	name, errs := NormalizeName(name)
	if err := errs.Err(); err != nil {
		return Folder{}, err
	}
	var out Folder
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		id, err := store.New(tx).InsertFolder(ctx, store.InsertFolderParams{Kind: kind, Name: name})
		if err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "folder.created", EntityType: "folder", EntityID: id,
			Changes: map[string]any{"kind": map[string]any{"to": kind}, "name": map[string]any{"to": name}}}); err != nil {
			return err
		}
		out, err = load(ctx, tx, p, id)
		return err
	})
	return out, err
}

// Rename renames a folder (its owner), at the version they loaded.
func (s *Service) Rename(ctx context.Context, p authz.Principal, id, name string, version int) (Folder, error) {
	name, errs := NormalizeName(name)
	if err := errs.Err(); err != nil {
		return Folder{}, err
	}
	var out Folder
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		before, err := owned(ctx, tx, p, id)
		if err != nil {
			return err
		}
		n, err := store.New(tx).RenameFolder(ctx, store.RenameFolderParams{ID: id, Name: name, Version: int32(version)}) //nolint:gosec // a version number
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.StaleVersion("folder")
		}
		if err := audit.Record(ctx, tx, audit.Entry{Action: "folder.renamed", EntityType: "folder", EntityID: id,
			Changes: map[string]any{"name": map[string]any{"from": before.Name, "to": name}}}); err != nil {
			return err
		}
		out, err = load(ctx, tx, p, id)
		return err
	})
	return out, err
}

// Delete deletes a folder (its owner); the records in it stay as they are.
func (s *Service) Delete(ctx context.Context, p authz.Principal, id string) error {
	return database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		f, err := owned(ctx, tx, p, id)
		if err != nil {
			return err
		}
		if _, err := store.New(tx).DeleteFolder(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{Action: "folder.deleted", EntityType: "folder", EntityID: id,
			Changes: map[string]any{"name": map[string]any{"from": f.Name}, "items": map[string]any{"from": f.ItemCount}}})
	})
}

// Share shares a folder (its owner) with an active user of the same agency.
func (s *Service) Share(ctx context.Context, p authz.Principal, id, userID string) (Folder, error) {
	var out Folder
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		f, err := held(ctx, tx, p, id)
		if err == nil && !f.Mine {
			err = OwnerOnly()
		}
		if err != nil {
			return err
		}
		if userID == f.OwnerID {
			return apperr.Validation(apperr.FieldErrors{{Field: "userId", Message: "is the folder's owner"}})
		}
		ok, err := s.users.Active(ctx, tx, userID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.NotFound("user")
		}
		q := store.New(tx)
		n, err := q.AddShare(ctx, store.AddShareParams{FolderID: id, UserID: userID})
		if err != nil {
			return err
		}
		if n > 0 {
			if err := q.TouchFolder(ctx, id); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: "folder.shared", EntityType: "folder", EntityID: id,
				Changes: map[string]any{"sharedWith": map[string]any{"added": userID}}}); err != nil {
				return err
			}
		}
		out, err = load(ctx, tx, p, id)
		return err
	})
	return out, err
}

// Unshare stops sharing a folder with someone (its owner), or lets someone
// leave a folder shared with them; then it returns nil (they no longer see it).
func (s *Service) Unshare(ctx context.Context, p authz.Principal, id, userID string) (*Folder, error) {
	var out *Folder
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		f, err := load(ctx, tx, p, id)
		if err != nil {
			return err
		}
		if !f.Mine && userID != p.UserID {
			return OwnerOnly()
		}
		q := store.New(tx)
		n, err := q.RemoveShare(ctx, store.RemoveShareParams{FolderID: id, UserID: userID})
		if err != nil {
			return err
		}
		if n > 0 {
			if err := q.TouchFolder(ctx, id); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: "folder.unshared", EntityType: "folder", EntityID: id,
				Changes: map[string]any{"sharedWith": map[string]any{"removed": userID}}}); err != nil {
				return err
			}
		}
		if !f.Mine {
			return nil // they left it
		}
		after, err := load(ctx, tx, p, id)
		out = &after
		return err
	})
	return out, err
}

// AddItems puts records in a folder (its owner, or those it is shared with).
func (s *Service) AddItems(ctx context.Context, p authz.Principal, id string, ids []string) (Folder, error) {
	return s.changeItems(ctx, p, id, ids, true)
}

// RemoveItems takes records out of a folder; the records stay as they are.
func (s *Service) RemoveItems(ctx context.Context, p authz.Principal, id string, ids []string) (Folder, error) {
	return s.changeItems(ctx, p, id, ids, false)
}

func (s *Service) changeItems(ctx context.Context, p authz.Principal, id string, ids []string, add bool) (Folder, error) {
	var out Folder
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		f, err := held(ctx, tx, p, id)
		if err != nil {
			return err
		}
		ids, errs := ItemIDs(f.Kind, ids)
		if err := errs.Err(); err != nil {
			return err
		}
		q := store.New(tx)
		var changed []string
		action := "folder.items_added"
		if add {
			// Only records that exist and that they may see go in.
			if ids, err = s.records.Visible(ctx, tx, f.Kind, ids); err != nil {
				return err
			}
			if len(ids) == 0 {
				return apperr.NotFound(f.Kind)
			}
			changed, err = q.AddItems(ctx, store.AddItemsParams{FolderID: id, ItemIds: ids})
		} else {
			action = "folder.items_removed"
			changed, err = q.RemoveItems(ctx, store.RemoveItemsParams{FolderID: id, ItemIds: ids})
		}
		if err != nil {
			return err
		}
		if len(changed) > 0 {
			if err := q.TouchFolder(ctx, id); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Entry{Action: action, EntityType: "folder", EntityID: id,
				Changes: map[string]any{"items": map[string]any{"ids": changed, "count": len(changed)}}}); err != nil {
				return err
			}
		}
		out, err = load(ctx, tx, p, id)
		return err
	})
	return out, err
}

// Membership is one folder holding some of the records asked about.
type Membership struct {
	FolderID string
	ItemIDs  []string
}

// Memberships says which of the signed-in person's folders hold each record.
func (s *Service) Memberships(ctx context.Context, p authz.Principal, kind string, ids []string) ([]Membership, error) {
	ids, errs := ItemIDs(kind, ids)
	if err := errs.Err(); err != nil {
		return nil, err
	}
	var out []Membership
	err := database.InTx(ctx, s.pool, p, func(tx pgx.Tx) error {
		rows, err := store.New(tx).Memberships(ctx, store.MembershipsParams{Kind: kind, ItemIds: ids})
		if err != nil {
			return err
		}
		out = make([]Membership, len(rows))
		for i, r := range rows {
			out[i] = Membership{FolderID: r.FolderID, ItemIDs: r.ItemIds}
		}
		return nil
	})
	return out, err
}

func fromRow(r store.VisibleFoldersRow, p authz.Principal) Folder {
	return Folder{ID: r.ID, Kind: r.Kind, Name: r.Name, OwnerID: r.OwnerID, Mine: r.OwnerID == p.UserID, SharedWith: r.SharedWith,
		ItemCount: int(r.ItemCount), Version: int(r.Version), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
