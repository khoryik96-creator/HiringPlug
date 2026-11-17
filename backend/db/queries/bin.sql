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
SELECT * FROM bin_entry WHERE id = @id;

-- name: DeleteBinEntry :execrows
DELETE FROM bin_entry WHERE id = @id;

-- The candidates and jobs in the Bin now (an application stays hidden while either is).
-- name: BinnedRecordIDs :many
SELECT record_id AS id FROM bin_entry WHERE kind = 'candidate'
UNION ALL
SELECT jsonb_array_elements_text(went_with -> 'jobs') FROM bin_entry WHERE kind = 'company';
