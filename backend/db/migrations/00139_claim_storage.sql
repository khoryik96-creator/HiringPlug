-- claims module: where submitted receipts are uploaded. One company account
-- (Google Drive, OneDrive or Dropbox), connected in Settings by an owner or
-- admin; each submitted claim's receipt is uploaded into
-- <root>/<month>/<person>/<receipt type>/.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE claim_storage (
  tenant_id     text PRIMARY KEY REFERENCES tenant (id),
  provider      text CHECK (provider IN ('google_drive', 'onedrive', 'dropbox')),
  account       text CHECK (length(account) <= 320),
  root_folder   text NOT NULL DEFAULT 'Claims' CHECK (length(root_folder) BETWEEN 1 AND 200),
  -- The provider's tokens, sealed (AES-GCM, HP_MAIL_KEYS_SECRET): never stored readable.
  sealed_tokens text,
  app_client_id text,
  status        text NOT NULL DEFAULT 'disconnected' CHECK (status IN ('disconnected', 'ok', 'reconnect')),
  error         text CHECK (length(error) <= 500),
  connected_at  timestamptz,
  connected_by  text,
  version       integer NOT NULL DEFAULT 1,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  CHECK ((provider IS NULL) = (sealed_tokens IS NULL))
);

-- A sign-in in progress (one per person): the PKCE verifier, then the code
-- the provider sent back, which only the person who started it can use.
CREATE TABLE claim_storage_state (
  tenant_id       text NOT NULL REFERENCES tenant (id),
  user_id         text NOT NULL,
  provider        text NOT NULL CHECK (provider IN ('google_drive', 'onedrive', 'dropbox')),
  state_hash      text NOT NULL UNIQUE,
  app_client_id   text NOT NULL,
  sealed_verifier text NOT NULL,
  sealed_code     text,
  ticket_hash     text,
  consumed        boolean NOT NULL DEFAULT false,
  expires_at      timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, user_id),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);

-- Each claim's upload: where it went, so submitting it again replaces it.
ALTER TABLE claim
  ADD COLUMN upload_status    text NOT NULL DEFAULT 'none' CHECK (upload_status IN ('none', 'pending', 'done', 'failed')),
  ADD COLUMN upload_sequence  integer NOT NULL DEFAULT 0,
  ADD COLUMN upload_path      text CHECK (length(upload_path) <= 1000),
  ADD COLUMN upload_url       text CHECK (length(upload_url) <= 2000),
  ADD COLUMN upload_error     text CHECK (length(upload_error) <= 500),
  ADD COLUMN uploaded_at      timestamptz,
  ADD COLUMN upload_provider  text CHECK (upload_provider IN ('google_drive', 'onedrive', 'dropbox')),
  ADD COLUMN upload_account   text CHECK (length(upload_account) <= 320),
  ADD COLUMN upload_remote_id text CHECK (length(upload_remote_id) <= 500);
CREATE INDEX claim_upload_idx ON claim (tenant_id, upload_status) WHERE upload_status IN ('pending', 'failed');

-- Only owners and admins (claims.storage:manage) see or change the storage;
-- the worker uploads as the system.
ALTER TABLE claim_storage ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON claim_storage AS RESTRICTIVE
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY claim_storage_manage ON claim_storage
  USING (app_has_permission('claims.storage:manage')) WITH CHECK (app_has_permission('claims.storage:manage'));

ALTER TABLE claim_storage_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON claim_storage_state AS RESTRICTIVE
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY claim_storage_state_own ON claim_storage_state
  USING (user_id = app_current_user() AND app_has_permission('claims.storage:manage'))
  WITH CHECK (user_id = app_current_user() AND app_has_permission('claims.storage:manage'));

UPDATE role_setting SET permissions = array_append(permissions, 'claims.storage:manage')
  WHERE role IN ('owner', 'admin') AND NOT ('claims.storage:manage' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'claims.storage:manage');
ALTER TABLE claim
  DROP COLUMN upload_status, DROP COLUMN upload_sequence, DROP COLUMN upload_path, DROP COLUMN upload_url,
  DROP COLUMN upload_error, DROP COLUMN uploaded_at, DROP COLUMN upload_provider, DROP COLUMN upload_account,
  DROP COLUMN upload_remote_id;
DROP TABLE IF EXISTS claim_storage_state;
DROP TABLE IF EXISTS claim_storage;
