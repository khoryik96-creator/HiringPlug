-- Backup download links: a random token kept (hashed) by the server until it
-- expires, so a link cannot be made from a secret key alone. One table for
-- the whole server, like backup_run: not company data.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE backup_download (
  token_hash text PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
  name       text NOT NULL CHECK (name ~ '^hiringplug-[0-9]{8}-[0-9]{6}$'),
  expires_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS backup_download;
