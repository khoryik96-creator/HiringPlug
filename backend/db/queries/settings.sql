-- Queries owned by the settings module. Run inside a tenant-scoped transaction.

-- name: ListOptions :many
SELECT list, key, label, hint, hidden FROM field_option ORDER BY list, position;

-- Lists the company has saved (their rows are the whole list).
-- name: SavedLists :many
SELECT list FROM field_list_saved;

-- name: DeleteOptions :exec
DELETE FROM field_option WHERE list = @list;

-- name: InsertOption :exec
INSERT INTO field_option (tenant_id, list, key, label, hint, hidden, position)
VALUES (app_current_tenant(), @list, @key, @label, sqlc.narg(hint), @hidden, @position);

-- name: MarkSaved :exec
INSERT INTO field_list_saved (tenant_id, list, updated_by)
VALUES (app_current_tenant(), @list, app_current_user())
ON CONFLICT (tenant_id, list) DO UPDATE SET updated_by = excluded.updated_by, updated_at = now();

-- name: ListRules :many
SELECT form, field, mandatory FROM field_rule;

-- name: PutRule :exec
INSERT INTO field_rule (tenant_id, form, field, mandatory)
VALUES (app_current_tenant(), @form, @field, @mandatory)
ON CONFLICT (tenant_id, form, field) DO UPDATE SET mandatory = excluded.mandatory;

-- name: GetPolicy :one
SELECT value FROM tenant_policy WHERE key = @key;

-- name: SetPolicy :exec
INSERT INTO tenant_policy (tenant_id, key, value) VALUES (app_current_tenant(), @key, @value)
ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value;
