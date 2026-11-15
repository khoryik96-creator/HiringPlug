-- Each backup is also copied off the server (to a storage account, by
-- `hp backup`): whether that copy went through, shown in Settings > System
-- and checked by the alerts. 'off' when no off-site storage is set up.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE backup_run
  ADD COLUMN offsite_status text CHECK (offsite_status IN ('done', 'failed', 'off')),
  ADD COLUMN offsite_at timestamptz,
  ADD COLUMN offsite_error text;

-- +goose Down
ALTER TABLE backup_run DROP COLUMN offsite_status, DROP COLUMN offsite_at, DROP COLUMN offsite_error;
