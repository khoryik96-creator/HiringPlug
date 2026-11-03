-- Queries owned by the cvformats module. Run inside a tenant-scoped transaction.

-- name: ListTemplates :many
SELECT * FROM cv_template ORDER BY lower(name), id LIMIT 100;

-- name: GetTemplate :one
SELECT * FROM cv_template WHERE id = @id;

-- name: NewTemplateID :one
SELECT new_id('ctpl')::text;

-- name: InsertTemplate :one
INSERT INTO cv_template (id, tenant_id, name, storage_key, uploaded_by)
VALUES (@id, app_current_tenant(), @name, @storage_key, app_current_user())
RETURNING *;

-- name: DeleteTemplate :one
DELETE FROM cv_template WHERE id = @id RETURNING storage_key;
