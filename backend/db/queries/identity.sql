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

-- microsoft_required: the company requires Microsoft (passwords only for owners).
-- name: UserAuth :one
SELECT u.id, u.tenant_id, u.email, u.name, u.role, u.team_id, u.active, u.invited, u.password_hash, u.tokens_valid_after,
  coalesce((SELECT t.microsoft_required FROM tenant t WHERE t.id = u.tenant_id), false)::boolean AS microsoft_required
FROM app_user u WHERE u.id = @id;

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
SELECT id, email, name, role, team_id, active, invited, last_sign_in_at, created_at FROM app_user ORDER BY active DESC, lower(name) LIMIT 1000;

-- name: InviteUser :one
INSERT INTO app_user (tenant_id, email, name, role, active, invited)
VALUES (app_current_tenant(), @email, @name, @role, false, true)
RETURNING id, email, name, role, team_id, active, invited, last_sign_in_at, created_at;

-- name: TeamMember :one
SELECT id, email, name, role, team_id, active, invited, last_sign_in_at, created_at FROM app_user WHERE id = @id;

-- Deactivating signs the user out: tokens issued up to the end of this second
-- stop working (tokens carry whole seconds, like sessionsEndedAt in auth.go).
-- name: UpdateMember :one
UPDATE app_user SET role = @role, active = @active, team_id = sqlc.narg(team_id),
  tokens_valid_after = CASE WHEN active AND NOT @active THEN date_trunc('second', now()) + interval '1 second' ELSE tokens_valid_after END
WHERE id = @id
RETURNING id, email, name, role, team_id, active, invited, last_sign_in_at, created_at;

-- Cancelling an invitation: only someone who never joined is removed (their
-- links go with them).
-- name: DeleteInvited :execrows
DELETE FROM app_user WHERE id = @id AND invited;

-- name: ActiveOwners :one
SELECT count(*) FROM app_user WHERE role = 'owner' AND active;

-- A company's own settings for one role (none: the contract's defaults).
-- name: GetRoleSetting :one
SELECT * FROM role_setting WHERE role = @role;

-- name: ListRoleSettings :many
SELECT * FROM role_setting ORDER BY role;

-- name: InsertRoleSetting :one
INSERT INTO role_setting (tenant_id, role, permissions, known_permissions, candidate_scope, job_candidates, updated_by)
VALUES (app_current_tenant(), @role, @permissions, @known_permissions, @candidate_scope, @job_candidates, app_current_user())
ON CONFLICT (tenant_id, role) DO NOTHING
RETURNING *;

-- Optimistic concurrency: no row returned = stale version.
-- name: UpdateRoleSetting :one
UPDATE role_setting
SET permissions = @permissions, known_permissions = @known_permissions, candidate_scope = @candidate_scope, job_candidates = @job_candidates,
    updated_by = app_current_user(), version = version + 1, updated_at = now()
WHERE role = @role AND version = @version
RETURNING *;

-- name: DeleteRoleSetting :exec
DELETE FROM role_setting WHERE role = @role;

-- The active users in a team (whose candidates its members see with the team scope).
-- name: TeamUserIDs :many
SELECT id FROM app_user WHERE team_id = @team_id AND active ORDER BY id;

-- name: ListTeamGroups :many
SELECT g.id, g.version, g.name, (SELECT count(*) FROM app_user u WHERE u.team_id = g.id AND u.active)::integer AS member_count
FROM team_group g ORDER BY lower(g.name) LIMIT 500;

-- name: GetTeamGroup :one
SELECT id, version, name FROM team_group WHERE id = @id;

-- name: InsertTeamGroup :one
INSERT INTO team_group (tenant_id, name, created_by) VALUES (app_current_tenant(), @name, app_current_user())
RETURNING id, version, name;

-- name: RenameTeamGroup :one
UPDATE team_group SET name = @name, version = version + 1, updated_at = now()
WHERE id = @id AND version = @version
RETURNING id, version, name;

-- Its people stay, in no team (the foreign key clears their team_id).
-- name: DeleteTeamGroup :execrows
DELETE FROM team_group WHERE id = @id;

-- name: RevokeSession :exec
INSERT INTO revoked_session (id, tenant_id, user_id, expires_at)
VALUES (@id, app_current_tenant(), @user_id, @expires_at)
ON CONFLICT (id) DO NOTHING;

-- name: PruneRevokedSessions :exec
DELETE FROM revoked_session WHERE expires_at < now();

-- name: SessionRevoked :one
SELECT EXISTS (SELECT 1 FROM revoked_session WHERE id = @id);

-- The signed-in person's own settings (none until they change one).
-- name: GetPreferences :one
SELECT candidate_columns, dashboard_blocks, favourites, guide_dismissed FROM user_preference WHERE user_id = app_current_user();

-- name: SetCandidateColumns :exec
INSERT INTO user_preference (tenant_id, user_id, candidate_columns)
VALUES (app_current_tenant(), app_current_user(), @candidate_columns::text[])
ON CONFLICT (tenant_id, user_id) DO UPDATE SET candidate_columns = EXCLUDED.candidate_columns, updated_at = now();

-- NULL: back to the default order.
-- name: SetDashboardBlocks :exec
INSERT INTO user_preference (tenant_id, user_id, dashboard_blocks)
VALUES (app_current_tenant(), app_current_user(), sqlc.narg(dashboard_blocks)::text[])
ON CONFLICT (tenant_id, user_id) DO UPDATE SET dashboard_blocks = EXCLUDED.dashboard_blocks, updated_at = now();

-- name: SetFavourites :exec
INSERT INTO user_preference (tenant_id, user_id, favourites)
VALUES (app_current_tenant(), app_current_user(), @favourites::jsonb)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET favourites = EXCLUDED.favourites, updated_at = now();

-- The dashboard's getting-started checklist hidden (or shown again).
-- name: SetGuideDismissed :exec
INSERT INTO user_preference (tenant_id, user_id, guide_dismissed)
VALUES (app_current_tenant(), app_current_user(), @guide_dismissed::boolean)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET guide_dismissed = EXCLUDED.guide_dismissed, updated_at = now();

-- name: ResetCandidateColumns :exec
UPDATE user_preference SET candidate_columns = NULL, updated_at = now() WHERE user_id = app_current_user();

-- name: RenameTenant :one
UPDATE tenant SET name = @name WHERE id = app_current_tenant()
RETURNING id, name, careers_slug::text AS careers_slug;

-- Two-step sign-in -----------------------------------------------------------

-- name: GetTwoStep :one
SELECT * FROM user_two_step WHERE user_id = @user_id;

-- A new authenticator secret being added (an enabled one stays until confirmed).
-- name: SetTwoStepPending :exec
INSERT INTO user_two_step (tenant_id, user_id, pending_sealed) VALUES (app_current_tenant(), @user_id, @pending_sealed)
ON CONFLICT (tenant_id, user_id) DO UPDATE SET pending_sealed = EXCLUDED.pending_sealed, updated_at = now();

-- name: EnableTwoStep :execrows
UPDATE user_two_step SET secret_sealed = pending_sealed, pending_sealed = NULL, enabled_at = now(), last_step = @last_step,
  recovery_hashes = @recovery_hashes, updated_at = now()
WHERE user_id = @user_id AND pending_sealed IS NOT NULL;

-- A code is accepted once: only for a step after the last one used.
-- name: UseTwoStepStep :execrows
UPDATE user_two_step SET last_step = @step, updated_at = now() WHERE user_id = @user_id AND enabled_at IS NOT NULL AND last_step < @step;

-- name: UseRecoveryCode :execrows
UPDATE user_two_step SET recovery_hashes = array_remove(recovery_hashes, @hash::text), updated_at = now()
WHERE user_id = @user_id AND enabled_at IS NOT NULL AND @hash::text = ANY (recovery_hashes);

-- name: SetRecoveryCodes :exec
UPDATE user_two_step SET recovery_hashes = @recovery_hashes, updated_at = now() WHERE user_id = @user_id AND enabled_at IS NOT NULL;

-- name: DisableTwoStep :execrows
DELETE FROM user_two_step WHERE user_id = @user_id;

-- name: TwoStepUsers :many
SELECT user_id FROM user_two_step WHERE enabled_at IS NOT NULL;

-- name: TwoStepRequired :one
SELECT two_step_required FROM tenant WHERE id = app_current_tenant();

-- name: SetTwoStepRequired :exec
UPDATE tenant SET two_step_required = @required WHERE id = app_current_tenant();

-- Requiring two-step sign-in signs out everyone who has not set it up.
-- name: EndSessionsWithoutTwoStep :exec
UPDATE app_user SET tokens_valid_after = date_trunc('second', now()) + interval '1 second'
WHERE active AND id NOT IN (SELECT user_id FROM user_two_step WHERE enabled_at IS NOT NULL);

-- name: EndUserSessions :exec
UPDATE app_user SET tokens_valid_after = date_trunc('second', now()) + interval '1 second' WHERE id = @id;

-- Sign in with Microsoft ------------------------------------------------------

-- Before anyone is signed in (no company scope yet): app_microsoft_* return
-- only what routing and finishing a sign-in need (migration 00198).

-- name: SaveMicrosoftFlow :exec
SELECT app_microsoft_flow_save(@key_hash::text, @binding_hash::text, sqlc.narg(tenant_id)::text, @client_id::text,
  @authority::text, @sealed::text, @verified::boolean, sqlc.narg(ticket_hash)::text, @expires_at::timestamptz);

-- name: ParkMicrosoftFlow :one
SELECT app_microsoft_flow_park(@key_hash::text, @sealed_code::text, @ticket_hash::text)::integer AS parked;

-- name: TakeMicrosoftFlow :one
SELECT key_hash::text AS key_hash, binding_hash::text AS binding_hash, coalesce(tenant_id, '')::text AS tenant_id, client_id::text AS client_id, authority::text AS authority,
  sealed::text AS sealed, verified::boolean AS verified, coalesce(sealed_code, '')::text AS sealed_code
FROM app_microsoft_flow_take(@ticket_hash::text);

-- name: MicrosoftSignInTenants :many
SELECT tenant_id::text AS tenant_id, microsoft_tenant_id::text AS microsoft_tenant_id
FROM app_microsoft_sign_in_tenants(sqlc.narg(slug)::text);

-- name: MicrosoftAccounts :many
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name, careers_slug::text AS careers_slug,
  user_id::text AS user_id, linked::boolean AS linked
FROM app_microsoft_accounts(@ms_tenant_id::text, @ms_object_id::text, @emails::text[]);

-- The company's Microsoft sign-in settings.
-- name: MicrosoftSettings :one
SELECT microsoft_sign_in, microsoft_tenant_id, microsoft_required FROM tenant WHERE id = app_current_tenant();

-- name: SetMicrosoftSettings :one
UPDATE tenant SET microsoft_sign_in = @enabled, microsoft_tenant_id = sqlc.narg(microsoft_tenant_id), microsoft_required = @required
WHERE id = app_current_tenant()
RETURNING microsoft_sign_in, microsoft_tenant_id, microsoft_required;

-- name: LinkMicrosoft :exec
INSERT INTO user_microsoft (tenant_id, user_id, ms_tenant_id, ms_object_id)
VALUES (app_current_tenant(), @user_id, @ms_tenant_id, @ms_object_id);

-- name: MicrosoftLink :one
SELECT ms_tenant_id, ms_object_id FROM user_microsoft WHERE user_id = @user_id;

-- name: UsedMicrosoft :exec
UPDATE user_microsoft SET last_used_at = now() WHERE user_id = @user_id;

-- Accepting an invitation with Microsoft: active, without a password.
-- name: JoinWithMicrosoft :exec
UPDATE app_user SET active = true, invited = false WHERE id = @id AND invited;

-- How each person can sign in (for the team list).
-- name: SignInMethods :many
SELECT u.id, (u.password_hash IS NOT NULL)::boolean AS has_password,
  EXISTS (SELECT 1 FROM user_microsoft m WHERE m.user_id = u.id)::boolean AS microsoft
FROM app_user u;

-- name: HasPassword :one
SELECT (password_hash IS NOT NULL)::boolean AS has_password FROM app_user WHERE id = @id;
