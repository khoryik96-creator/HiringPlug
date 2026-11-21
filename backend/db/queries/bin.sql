-- bin module: what is in the Bin. Its transactions turn on app.show_binned
-- (ShowBinned) so they see the binned records the database hides from others.

-- name: ShowBinned :exec
SELECT set_config('app.show_binned', 'on', true);

-- name: InsertBinEntry :one
INSERT INTO bin_entry (tenant_id, kind, record_id, label, went_with, binned_by, binned_name, purge_after)
VALUES (app_current_tenant(), @kind, @record_id, @label, @went_with, app_current_user(), @binned_name, @purge_after)
RETURNING *;

-- name: ListBinEntries :many
SELECT * FROM bin_entry ORDER BY binned_at DESC, id DESC LIMIT 500;

-- name: GetBinEntry :one
SELECT * FROM bin_entry WHERE id = @id FOR UPDATE;

-- name: DeleteBinEntry :execrows
DELETE FROM bin_entry WHERE id = @id;

-- name: InsertDocumentBinEntry :one
INSERT INTO bin_entry (tenant_id,kind,record_id,label,binned_by,binned_name,purge_after)
VALUES (app_current_tenant(),'document',@record_id,@label,@binned_by,@binned_name,@purge_after)
RETURNING *;
