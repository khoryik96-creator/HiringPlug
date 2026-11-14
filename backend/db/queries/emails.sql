-- Queries owned by the emails module. Run inside a tenant-scoped transaction.

-- name: ListTemplates :many
SELECT * FROM email_template ORDER BY lower(name) LIMIT 200;

-- name: GetTemplate :one
SELECT * FROM email_template WHERE id = @id;

-- name: InsertTemplate :one
INSERT INTO email_template (tenant_id, name, subject, body, body_html, created_by)
VALUES (app_current_tenant(), @name, @subject, @body, sqlc.narg(body_html), app_current_user())
RETURNING *;

-- name: DeleteTemplate :execrows
DELETE FROM email_template WHERE id = @id;

-- The starter templates, added the first time a company opens its templates
-- (two people opening them at once add them once).
-- name: SeedTemplate :exec
INSERT INTO email_template (tenant_id, name, subject, body, created_by)
VALUES (app_current_tenant(), @name, @subject, @body, app_current_user())
ON CONFLICT DO NOTHING;

-- Optimistic concurrency: no row returned = stale version (or not found).
-- name: UpdateTemplate :one
UPDATE email_template SET name = @name, subject = @subject, body = @body, body_html = sqlc.narg(body_html),
  version = version + 1, updated_at = now()
WHERE id = @id AND version = @expected_version
RETURNING *;

-- name: InsertEmail :one
INSERT INTO email (tenant_id, candidate_id, application_id, contact_id, to_addresses, cc_addresses, bcc_addresses, subject, body,
                   body_html, salary, salary_restricted, fees_restricted, sent_by, sender_name, reply_to)
VALUES (app_current_tenant(), sqlc.narg(candidate_id), sqlc.narg(application_id), sqlc.narg(contact_id), @to_addresses::text[],
        @cc_addresses::text[], @bcc_addresses::text[], @subject, @body, sqlc.narg(body_html), @salary, @salary_restricted,
        @fees_restricted, app_current_user(), @sender_name, @reply_to)
RETURNING *;

-- name: SetEmailCalendar :exec
UPDATE email SET calendar = @calendar WHERE id = @id;

-- name: GetEmail :one
SELECT * FROM email WHERE id = @id;

-- Their own emails and the shortlists they were in.
-- name: ListCandidateEmails :many
SELECT e.* FROM email e
WHERE e.candidate_id = @candidate_id
   OR e.id IN (SELECT ec.email_id FROM email_candidate ec WHERE ec.candidate_id = @candidate_id::text)
ORDER BY e.id DESC LIMIT 200;

-- name: InsertEmailCandidate :exec
INSERT INTO email_candidate (tenant_id, email_id, candidate_id, application_id, position)
VALUES (app_current_tenant(), @email_id, @candidate_id, sqlc.narg(application_id), @position);

-- name: ListContactEmails :many
SELECT * FROM email WHERE contact_id = @contact_id ORDER BY id DESC LIMIT 200;

-- name: MarkEmail :exec
UPDATE email SET status = @status, error = sqlc.narg(error),
  sent_at = CASE WHEN @status = 'sent' THEN now() ELSE sent_at END
WHERE id = @id;

-- Merging candidates: the duplicate's emails follow them.
-- name: MoveEmailsToCandidate :execrows
UPDATE email SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- A shortlist that named both keeps the one kept, once.
-- name: DropShortlistDuplicates :exec
DELETE FROM email_candidate d WHERE d.candidate_id = @from_id
  AND EXISTS (SELECT 1 FROM email_candidate k WHERE k.email_id = d.email_id AND k.candidate_id = @to_id);

-- name: MoveShortlistsToCandidate :exec
UPDATE email_candidate SET candidate_id = @to_id WHERE candidate_id = @from_id;

-- name: InsertBanner :one
INSERT INTO email_banner (tenant_id, name, content_type, size_bytes, storage_key, created_by)
VALUES (app_current_tenant(), @name, @content_type, @size_bytes, @storage_key, app_current_user())
RETURNING *;

-- name: ListBanners :many
SELECT * FROM email_banner WHERE deleted_at IS NULL ORDER BY created_at, id LIMIT 100;

-- Banners an email shows, deleted ones too (they were in it when it was sent).
-- name: GetBanners :many
SELECT * FROM email_banner WHERE id = ANY(@ids::text[]);

-- name: DeleteBanner :execrows
UPDATE email_banner SET deleted_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: InsertAttachment :one
INSERT INTO email_attachment (tenant_id, email_id, file_name, content_type, size_bytes, storage_key, created_by)
VALUES (app_current_tenant(), sqlc.narg(email_id), @file_name, @content_type, @size_bytes, @storage_key, app_current_user())
RETURNING *;

-- Files the user uploaded and has not sent yet, among ids.
-- name: StagedAttachments :many
SELECT * FROM email_attachment WHERE id = ANY(@ids::text[]) AND email_id IS NULL AND created_by = app_current_user();

-- name: AttachToEmail :execrows
UPDATE email_attachment SET email_id = @email_id WHERE id = ANY(@ids::text[]) AND email_id IS NULL;

-- name: EmailAttachments :many
SELECT * FROM email_attachment WHERE email_id = ANY(@email_ids::text[]) ORDER BY created_at, id;

-- Uploads never sent, left for a day: removed (their files too).
-- name: DeleteStaleAttachments :many
DELETE FROM email_attachment WHERE email_id IS NULL AND created_at < now() - interval '1 day' RETURNING storage_key;

-- name: MailboxForUser :one
SELECT * FROM email_mailbox WHERE user_id = @user_id FOR UPDATE;
-- name: SaveMailbox :one
INSERT INTO email_mailbox(provider,address,host,port,tls,username,sealed_secret)
VALUES (@provider,@address,@host,@port,@tls,@username,@sealed_secret)
ON CONFLICT(tenant_id,user_id) DO UPDATE SET id=new_id('mbx'),provider=excluded.provider,address=excluded.address,
 host=excluded.host,port=excluded.port,tls=excluded.tls,username=excluded.username,sealed_secret=excluded.sealed_secret,version=email_mailbox.version+1,updated_at=now()
RETURNING *;
-- name: DeleteMailbox :exec
DELETE FROM email_mailbox WHERE user_id=app_current_user();
-- name: RefreshMailboxSecret :exec
UPDATE email_mailbox SET sealed_secret = @sealed_secret,updated_at=now() WHERE id = @id;
-- name: SaveMailboxState :exec
INSERT INTO email_mailbox_state(state_hash,provider,sealed_verifier,expires_at) VALUES (@state_hash,@provider,@sealed_verifier,@expires_at)
ON CONFLICT(tenant_id,user_id) DO UPDATE SET state_hash=excluded.state_hash,provider=excluded.provider,sealed_verifier=excluded.sealed_verifier,expires_at=excluded.expires_at,consumed=false;
-- name: ConsumeMailboxState :one
UPDATE email_mailbox_state SET consumed=true WHERE state_hash = @state_hash AND expires_at>now() AND NOT consumed RETURNING *;
-- name: ClearMailboxStates :exec
DELETE FROM email_mailbox_state WHERE user_id=app_current_user();
-- name: BindEmailDelivery :exec
UPDATE email SET mailbox_ref=sqlc.narg(mailbox_ref),delivery_mode = @delivery_mode WHERE id = @id;

-- name: FinishMailboxState :execrows
DELETE FROM email_mailbox_state WHERE state_hash = @state_hash AND consumed;
-- name: LockMailboxUser :exec
SELECT pg_advisory_xact_lock(hashtextextended(app_current_tenant() || ':' || app_current_user(),0));
-- name: MailboxForDelivery :one
SELECT * FROM email_mailbox WHERE user_id = @user_id;

-- The agency's own app details for a mail provider (Settings > Email connection).
-- name: MailApps :many
SELECT * FROM email_mail_app ORDER BY provider;

-- name: MailApp :one
SELECT * FROM email_mail_app WHERE provider = @provider;

-- name: SaveMailApp :exec
INSERT INTO email_mail_app (tenant_id, provider, client_id, sealed_secret, secret_ending, updated_by)
VALUES (app_current_tenant(), @provider, @client_id, @sealed_secret, @secret_ending, app_current_user())
ON CONFLICT (tenant_id, provider) DO UPDATE SET client_id = EXCLUDED.client_id, sealed_secret = EXCLUDED.sealed_secret,
  secret_ending = EXCLUDED.secret_ending, updated_by = EXCLUDED.updated_by, updated_at = now();

-- name: DeleteMailApp :exec
DELETE FROM email_mail_app WHERE provider = @provider;

-- name: MarkBounced :exec
UPDATE email SET bounced_to = @bounced_to, bounced_at = coalesce(bounced_at, now()) WHERE id = @id;
