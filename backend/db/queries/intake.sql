-- Queries owned by the intake module. Run inside a tenant-scoped transaction.

-- name: InsertImport :one
INSERT INTO cv_import (tenant_id, source, created_by) VALUES (app_current_tenant(), @source, app_current_user())
RETURNING *;

-- name: GetImport :one
SELECT * FROM cv_import WHERE id = @id AND created_by = app_current_user();

-- The user's latest imports.
-- name: ListImports :many
SELECT * FROM cv_import WHERE created_by = app_current_user() ORDER BY created_at DESC, id DESC LIMIT 30;

-- name: InsertItem :one
INSERT INTO cv_import_item (tenant_id, import_id, file_name, document_id, candidate_id, status, error)
VALUES (app_current_tenant(), @import_id, @file_name, sqlc.narg(document_id), sqlc.narg(candidate_id), @status, sqlc.narg(error))
RETURNING *;

-- name: ListItems :many
SELECT * FROM cv_import_item WHERE import_id = ANY(@import_ids::text[]) ORDER BY created_at, id;

-- The import file a document came from, while it is being read.
-- name: ReadingItemByDocument :one
SELECT * FROM cv_import_item WHERE document_id = @document_id AND status = 'reading';

-- name: FinishItem :exec
UPDATE cv_import_item SET status = @status, candidate_id = sqlc.narg(candidate_id), error = sqlc.narg(error), updated_at = now()
WHERE id = @id;
