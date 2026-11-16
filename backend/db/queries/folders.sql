-- Queries owned by the folders module. Run inside a tenant-scoped transaction.
-- A folder is visible to its owner and to those it is shared with.

-- name: InsertFolder :one
INSERT INTO folder (tenant_id, kind, name, owner_id)
VALUES (app_current_tenant(), @kind, @name, app_current_user())
RETURNING id;

-- name: VisibleFolders :many
SELECT f.id, f.kind, f.name, f.owner_id, f.version, f.created_at, f.updated_at,
       (SELECT count(*) FROM folder_item i WHERE i.folder_id = f.id)::integer AS item_count,
       coalesce((SELECT array_agg(s.user_id ORDER BY s.created_at) FROM folder_share s WHERE s.folder_id = f.id), '{}')::text[] AS shared_with
FROM folder f
WHERE f.kind = @kind
  AND (f.owner_id = app_current_user()
       OR EXISTS (SELECT 1 FROM folder_share s WHERE s.folder_id = f.id AND s.user_id = app_current_user()))
ORDER BY lower(f.name), f.id;

-- name: VisibleFolder :one
SELECT f.id, f.kind, f.name, f.owner_id, f.version, f.created_at, f.updated_at,
       (SELECT count(*) FROM folder_item i WHERE i.folder_id = f.id)::integer AS item_count,
       coalesce((SELECT array_agg(s.user_id ORDER BY s.created_at) FROM folder_share s WHERE s.folder_id = f.id), '{}')::text[] AS shared_with
FROM folder f
WHERE f.id = @id
  AND (f.owner_id = app_current_user()
       OR EXISTS (SELECT 1 FROM folder_share s WHERE s.folder_id = f.id AND s.user_id = app_current_user()));

-- Holds a folder while it changes (records, people, deletion): changes to one
-- folder take turns, so a delete never cuts an addition off halfway and two
-- additions never wait on each other's lock (a deadlock with FOR SHARE, as
-- each then updates the row).
-- name: LockFolder :one
SELECT id FROM folder WHERE id = @id FOR NO KEY UPDATE;

-- The kind of a folder the signed-in person may see (owner or shared), for a
-- list's filter: cheaper than VisibleFolder, which counts its records.
-- name: VisibleFolderKind :one
SELECT f.kind FROM folder f
WHERE f.id = @id
  AND (f.owner_id = app_current_user()
       OR EXISTS (SELECT 1 FROM folder_share s WHERE s.folder_id = f.id AND s.user_id = app_current_user()));

-- name: RenameFolder :execrows
UPDATE folder SET name = @name, version = version + 1, updated_at = now()
WHERE id = @id AND version = @version;

-- name: DeleteFolder :execrows
DELETE FROM folder WHERE id = @id;

-- Records and people coming and going do not change the folder itself (its
-- version), so they never make the owner's rename stale.
-- name: TouchFolder :exec
UPDATE folder SET updated_at = now() WHERE id = @id;

-- name: AddShare :execrows
INSERT INTO folder_share (folder_id, tenant_id, user_id)
VALUES (@folder_id, app_current_tenant(), @user_id)
ON CONFLICT DO NOTHING;

-- name: RemoveShare :execrows
DELETE FROM folder_share WHERE folder_id = @folder_id AND user_id = @user_id;

-- AddItems and RemoveItems return the records actually put in or taken out.
-- name: AddItems :many
INSERT INTO folder_item (folder_id, tenant_id, item_id, added_by)
SELECT @folder_id, app_current_tenant(), unnest(@item_ids::text[]), app_current_user()
ON CONFLICT DO NOTHING
RETURNING item_id;

-- name: RemoveItems :many
DELETE FROM folder_item WHERE folder_id = @folder_id AND item_id = ANY (@item_ids::text[])
RETURNING item_id;

-- name: FolderItemIDs :many
SELECT item_id FROM folder_item WHERE folder_id = @folder_id ORDER BY added_at DESC, item_id;

-- name: Memberships :many
SELECT i.folder_id, array_agg(i.item_id ORDER BY i.item_id)::text[] AS item_ids
FROM folder_item i
JOIN folder f ON f.id = i.folder_id
WHERE f.kind = @kind AND i.item_id = ANY (@item_ids::text[])
  AND (f.owner_id = app_current_user()
       OR EXISTS (SELECT 1 FROM folder_share s WHERE s.folder_id = f.id AND s.user_id = app_current_user()))
GROUP BY i.folder_id;

-- A merged duplicate's folders follow the record kept (once in each folder):
-- MoveItems, then DropItem for the folders that held both.
-- name: MoveItems :exec
UPDATE folder_item AS fi SET item_id = @to_id
WHERE fi.item_id = @from_id
  AND NOT EXISTS (SELECT 1 FROM folder_item o WHERE o.folder_id = fi.folder_id AND o.item_id = @to_id);

-- name: DropItem :exec
DELETE FROM folder_item WHERE item_id = @item_id;
