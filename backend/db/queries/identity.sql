-- Queries owned by the identity module. Run inside a tenant-scoped transaction.

-- name: GetUser :one
SELECT id, tenant_id, email, name, role, active FROM app_user WHERE id = @id;

-- name: GetTenant :one
SELECT id, name, careers_slug::text AS careers_slug FROM tenant WHERE id = @id;

-- name: UserNames :many
SELECT id, name FROM app_user WHERE id = ANY(@ids::text[]);

-- The company behind a public careers page address (no tenant scope yet:
-- app_careers_tenant returns only the company's id and name).
-- name: CareersTenant :one
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name FROM app_careers_tenant(@slug::text);

-- name: ListActiveUsers :many
SELECT id, name, role FROM app_user WHERE active ORDER BY name LIMIT 500;

-- Which of these users are active in the tenant (to check interviewers and owners).
-- name: ActiveUserRoles :many
SELECT id, role FROM app_user WHERE active AND id = ANY(@ids::text[]);

-- Active users' names and addresses (to email interviewers).
-- name: ActiveUserEmails :many
SELECT id, name, email FROM app_user WHERE active AND id = ANY(@ids::text[]);

-- name: AccountsByEmail :many
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name, careers_slug::text AS careers_slug, user_id::text AS user_id,
  user_name::text AS user_name, coalesce(password_hash, '')::text AS password_hash, active::boolean AS active
FROM app_accounts_by_email(sqlc.arg(email)::text);

-- name: LinkByHash :one
SELECT link_id::text AS link_id, tenant_id::text AS tenant_id, user_id::text AS user_id, kind::text AS kind
FROM app_user_link(sqlc.arg(token_hash)::text);

-- name: UserAuth :one
SELECT id, tenant_id, email, name, role, active, invited, password_hash, tokens_valid_after FROM app_user WHERE id = @id;

-- name: SignedIn :exec
UPDATE app_user SET last_sign_in_at = now() WHERE id = @id;

-- A new password; sessions from before it are signed out.
-- name: SetPassword :exec
UPDATE app_user SET password_hash = sqlc.narg(password_hash), tokens_valid_after = sqlc.narg(valid_after),
  active = CASE WHEN invited THEN true ELSE active END, invited = false,
  name = coalesce(sqlc.narg(name), name)
WHERE id = @id;

-- name: InsertLink :one
INSERT INTO user_link (tenant_id, user_id, kind, token_hash, expires_at, created_by)
VALUES (app_current_tenant(), @user_id, @kind, @token_hash, @expires_at, app_current_user())
RETURNING id;

-- Using a link, or replacing it with a new one, ends it.
-- name: EndLinks :exec
UPDATE user_link SET used_at = now() WHERE user_id = @user_id AND kind = @kind AND used_at IS NULL;

-- name: ListTeam :many
SELECT id, email, name, role, active, invited, last_sign_in_at, created_at FROM app_user ORDER BY active DESC, lower(name) LIMIT 1000;

-- name: InviteUser :one
INSERT INTO app_user (tenant_id, email, name, role, active, invited)
VALUES (app_current_tenant(), @email, @name, @role, false, true)
RETURNING id, email, name, role, active, invited, last_sign_in_at, created_at;

-- name: TeamMember :one
SELECT id, email, name, role, active, invited, last_sign_in_at, created_at FROM app_user WHERE id = @id;

-- Deactivating signs the user out (their tokens stop working).
-- name: UpdateMember :one
UPDATE app_user SET role = @role, active = @active,
  tokens_valid_after = CASE WHEN active AND NOT @active THEN date_trunc('second', now()) ELSE tokens_valid_after END
WHERE id = @id
RETURNING id, email, name, role, active, invited, last_sign_in_at, created_at;

-- name: ActiveOwners :one
SELECT count(*) FROM app_user WHERE role = 'owner' AND active;
