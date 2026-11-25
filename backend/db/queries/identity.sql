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

-- microsoft_required: the company requires Microsoft (passwords only for owners);
-- microsoft_sign_in: it signs in with Microsoft at all (else Microsoft sessions stop).
-- name: UserAuth :one
SELECT u.id, u.tenant_id, u.email, u.name, u.role, u.team_id, u.active, u.invited, u.password_hash, u.tokens_valid_after,
  u.microsoft_sessions_after, u.microsoft_connect_only,
  coalesce((SELECT t.microsoft_required FROM tenant t WHERE t.id = u.tenant_id), false)::boolean AS microsoft_required,
  coalesce((SELECT t.microsoft_sign_in FROM tenant t WHERE t.id = u.tenant_id), false)::boolean AS microsoft_sign_in
FROM app_user u WHERE u.id = @id;

-- name: SignedIn :exec
UPDATE app_user SET last_sign_in_at = now() WHERE id = @id;

-- A new password; sessions from before it are signed out (never moving an
-- end already later back).
-- name: SetPassword :exec
UPDATE app_user SET password_hash = sqlc.narg(password_hash), tokens_valid_after = greatest(sqlc.narg(valid_after)::timestamptz, tokens_valid_after),
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

-- Everyone's unused links of a kind end (the company's rows are held first).
-- name: EndLinksOfKind :exec
UPDATE user_link SET used_at = now() WHERE kind = @kind AND used_at IS NULL;

-- name: ListTeam :many
SELECT id, email, name, role, team_id, active, invited, last_sign_in_at, created_at FROM app_user ORDER BY active DESC, lower(name) LIMIT 1000;

-- name: InviteUser :one
INSERT INTO app_user (tenant_id, email, name, role, active, invited)
VALUES (app_current_tenant(), @email, @name, @role, false, true)
RETURNING id, email, name, role, team_id, active, invited, last_sign_in_at, created_at;

-- name: TeamMember :one
SELECT id, email, name, role, team_id, active, invited, last_sign_in_at, created_at FROM app_user WHERE id = @id;

-- Deactivating signs the user out: tokens issued up to the end of this second
-- stop working (tokens carry whole seconds, like sessionsEndedAt in auth.go),
-- and any issued at an earlier end (a session is never issued before one).
-- This second is the clock's now (clock_timestamp), not the transaction's
-- start: a sign-in that committed while this waited for the row is ended
-- too. The row is held first (HoldUser): an UPDATE that waits for a row
-- computes the new values before it waits.
-- name: UpdateMember :one
UPDATE app_user SET role = @role, active = @active, team_id = sqlc.narg(team_id),
  tokens_valid_after = CASE WHEN active AND NOT @active THEN greatest(date_trunc('second', clock_timestamp()) + interval '1 second', tokens_valid_after + interval '1 second', microsoft_sessions_after + interval '1 second') ELSE tokens_valid_after END
WHERE id = @id
RETURNING id, email, name, role, team_id, active, invited, last_sign_in_at, created_at;

-- Cancelling an invitation: only someone who never joined is removed (their
-- links go with them).
-- name: DeleteInvited :execrows
DELETE FROM app_user WHERE id = @id AND invited;

-- name: ActiveOwners :one
SELECT count(*) FROM app_user WHERE role = 'owner' AND active;

-- Changes of the team's roles and activity, one at a time per company until
-- the transaction ends: taken before anything else is held (nothing else
-- takes it, so it never deadlocks), it keeps two changes from each counting
-- the other's owner (ActiveOwners) and leaving the company with none.
-- name: HoldTeamChanges :exec
SELECT pg_advisory_xact_lock(hashtextextended('team-changes:' || app_current_tenant(), 0));

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

-- Rows changed: 0 when it was so already.
-- name: SetTwoStepRequired :execrows
UPDATE tenant SET two_step_required = @required WHERE id = app_current_tenant() AND two_step_required <> @required;

-- Requiring two-step sign-in signs out everyone who has not set it up.
-- name: EndSessionsWithoutTwoStep :exec
UPDATE app_user SET tokens_valid_after = greatest(date_trunc('second', clock_timestamp()) + interval '1 second', tokens_valid_after + interval '1 second', microsoft_sessions_after + interval '1 second')
WHERE active AND id NOT IN (SELECT user_id FROM user_two_step WHERE enabled_at IS NOT NULL);

-- Ends every session of theirs: after this second by the database's clock,
-- and after every earlier end (a session issued at one is ended too).
-- name: EndUserSessions :exec
UPDATE app_user SET tokens_valid_after = greatest(date_trunc('second', clock_timestamp()) + interval '1 second', tokens_valid_after + interval '1 second', microsoft_sessions_after + interval '1 second') WHERE id = @id;

-- Sign in with Microsoft ------------------------------------------------------

-- Before anyone is signed in (no company scope yet): app_microsoft_* return
-- only what routing and finishing a sign-in need (migration 00198).

-- name: SaveMicrosoftFlow :exec
SELECT app_microsoft_flow_save(@key_hash::text, @binding_hash::text, sqlc.narg(tenant_id)::text, @purpose::text, sqlc.narg(user_id)::text,
  sqlc.narg(link_id)::text, sqlc.narg(link_kind)::text, @client_id::text, @authority::text, @sealed::text, @verified::boolean,
  sqlc.narg(ticket_hash)::text, @seconds::integer);

-- name: ParkMicrosoftFlow :one
SELECT app_microsoft_flow_park(@key_hash::text, @sealed_code::text, @ticket_hash::text)::integer AS parked;

-- name: TakeMicrosoftFlow :one
SELECT key_hash::text AS key_hash, binding_hash::text AS binding_hash, coalesce(tenant_id, '')::text AS tenant_id,
  purpose::text AS purpose, coalesce(user_id, '')::text AS user_id, coalesce(link_id, '')::text AS link_id,
  coalesce(link_kind, '')::text AS link_kind, client_id::text AS client_id, authority::text AS authority,
  sealed::text AS sealed, verified::boolean AS verified, coalesce(sealed_code, '')::text AS sealed_code
FROM app_microsoft_flow_take(@ticket_hash::text);

-- name: MicrosoftSignInTenants :many
SELECT tenant_id::text AS tenant_id, microsoft_tenant_id::text AS microsoft_tenant_id
FROM app_microsoft_sign_in_tenants(sqlc.narg(slug)::text);

-- name: MicrosoftAccounts :many
SELECT tenant_id::text AS tenant_id, tenant_name::text AS tenant_name, careers_slug::text AS careers_slug,
  user_id::text AS user_id, linked::boolean AS linked, is_owner::boolean AS is_owner, connect_only::boolean AS connect_only
FROM app_microsoft_accounts(@ms_tenant_id::text, @ms_object_id::text, @addresses::text[]);

-- Microsoft vouched, through the company's own app (client_id), for an
-- account of its organisation: whether the token said member or guest
-- (acct), which that app's settings in Entra decide. Without it, the app is
-- kept (Settings warns while sign-in uses that very app).
-- name: NoteMicrosoftClaims :exec
UPDATE tenant SET microsoft_acct_missing_app = CASE WHEN @acct::boolean THEN NULL ELSE @client_id::text END
WHERE id = app_current_tenant() AND microsoft_sign_in AND microsoft_tenant_id = @ms_tenant_id
  AND microsoft_acct_missing_app IS DISTINCT FROM (CASE WHEN @acct::boolean THEN NULL ELSE @client_id::text END);

-- The company's Microsoft sign-in settings.
-- name: MicrosoftSettings :one
SELECT microsoft_sign_in, microsoft_tenant_id, microsoft_required, microsoft_acct_missing_app FROM tenant WHERE id = app_current_tenant();

-- The company's settings, kept from changing until the transaction ends:
-- changing them (FOR NO KEY UPDATE), or the Microsoft app they rely on (FOR
-- SHARE, emails module), waits for the other. Not FOR UPDATE: that would
-- also stop every row naming the company being added (an audit entry of a
-- sign-in holding a person's row, which this then waits for: a deadlock).
-- name: LockMicrosoftSettings :exec
SELECT id FROM tenant WHERE id = app_current_tenant() FOR NO KEY UPDATE;

-- name: MicrosoftSignInHeld :one
SELECT microsoft_sign_in FROM tenant WHERE id = app_current_tenant() FOR SHARE;

-- name: SetMicrosoftSettings :one
UPDATE tenant SET microsoft_sign_in = @enabled, microsoft_tenant_id = sqlc.narg(microsoft_tenant_id), microsoft_required = @required,
  microsoft_acct_missing_app = CASE WHEN microsoft_tenant_id IS NOT DISTINCT FROM sqlc.narg(microsoft_tenant_id) THEN microsoft_acct_missing_app END
WHERE id = app_current_tenant()
RETURNING microsoft_sign_in, microsoft_tenant_id, microsoft_required;

-- A link to another organisation (left from before the company changed it)
-- makes way for a new one.
-- name: DropStaleMicrosoftLink :exec
DELETE FROM user_microsoft WHERE user_id = @user_id AND ms_tenant_id <> @ms_tenant_id;

-- name: LinkMicrosoft :exec
INSERT INTO user_microsoft (tenant_id, user_id, ms_tenant_id, ms_object_id, linked_by)
VALUES (app_current_tenant(), @user_id, @ms_tenant_id, @ms_object_id, @linked_by);

-- name: UnlinkMicrosoft :one
DELETE FROM user_microsoft WHERE user_id = @user_id RETURNING ms_tenant_id;

-- Their account was unlinked, or they were made an owner: one is linked
-- again only on purpose.
-- name: MicrosoftConnectOnly :exec
UPDATE app_user SET microsoft_connect_only = true WHERE id = @id;

-- Someone made an owner keeps no account linked by a sign-in's address.
-- name: UnlinkSignInLink :many
DELETE FROM user_microsoft WHERE user_id = @user_id AND linked_by = 'sign_in' RETURNING ms_tenant_id;

-- Who the Microsoft account is linked to here, if anyone.
-- name: MicrosoftAccountHolder :one
SELECT user_id FROM user_microsoft WHERE ms_tenant_id = @ms_tenant_id AND ms_object_id = @ms_object_id;

-- A link, held until the transaction ends (using it, or seeing it used,
-- waits for whoever holds it): whose, what for, and whether it still works.
-- FOR NO KEY UPDATE, not FOR UPDATE: a sign-in started from the link (its
-- microsoft_sign_in_flow names it) does not wait.
-- name: HoldLink :one
SELECT user_id, kind, (used_at IS NULL AND expires_at > now())::boolean AS valid FROM user_link WHERE id = @id FOR NO KEY UPDATE;

-- Holds the person's row (role, password, whether they may be linked by
-- address) until the transaction ends: a change to it waits, and so does
-- another sign-in of theirs. FOR NO KEY UPDATE, not FOR SHARE: the holder
-- updates the row afterwards (last sign-in, sessions ended), and two
-- holders sharing it would wait for each other. No row: no such person
-- here (pgx.ErrNoRows).
-- name: HoldUser :one
SELECT id FROM app_user WHERE id = @id FOR NO KEY UPDATE;

-- Everyone in the company, held as HoldUser does (in the same order, by
-- id), before links of many people are removed and their sessions ended.
-- name: HoldTeam :exec
SELECT id FROM app_user ORDER BY id FOR NO KEY UPDATE;

-- The database's clock, to the second, as of the transaction's start: when
-- a session is issued.
-- name: DatabaseSecond :one
SELECT date_trunc('second', now())::timestamptz AS now;

-- The database's clock now, to the second (clock_timestamp: not the
-- transaction's start, which may be long before, if it waited for a row):
-- where an end of sessions is cut off (tokens_valid_after,
-- microsoft_sessions_after), so that a session issued before it, even by a
-- sign-in that committed while this transaction waited, is ended.
-- name: CutOffSecond :one
SELECT date_trunc('second', clock_timestamp())::timestamptz AS now;

-- The company changed its organisation: links to any other go.
-- name: UnlinkOtherOrganisations :execrows
DELETE FROM user_microsoft WHERE ms_tenant_id <> coalesce(sqlc.narg(microsoft_tenant_id)::text, '');

-- Ends sessions signed in with Microsoft until now (the next whole second,
-- by the database's clock): one person's, or (no user) everyone's. Always
-- later than every earlier end, of these or of all their sessions: a session
-- signed in since (dated that end, in the same second) ends too.
-- name: EndMicrosoftSessions :exec
UPDATE app_user SET microsoft_sessions_after = greatest(date_trunc('second', clock_timestamp()) + interval '1 second', tokens_valid_after + interval '1 second', microsoft_sessions_after + interval '1 second')
WHERE sqlc.narg(user_id)::text IS NULL OR id = sqlc.narg(user_id)::text;

-- Whether the company signs in with Microsoft, and whether a Microsoft account is linked to them.
-- name: MyMicrosoft :one
SELECT coalesce((SELECT microsoft_sign_in FROM tenant WHERE id = app_current_tenant()), false)::boolean AS sign_in,
  EXISTS (SELECT 1 FROM user_microsoft WHERE user_id = @user_id)::boolean AS linked;

-- The account linked to them is still this one (it may have been unlinked
-- meanwhile: 0 rows).
-- name: UsedMicrosoft :execrows
UPDATE user_microsoft SET last_used_at = now() WHERE user_id = @user_id AND ms_tenant_id = @ms_tenant_id AND ms_object_id = @ms_object_id;

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
