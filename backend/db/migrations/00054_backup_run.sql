-- Backups of the whole server (database and uploaded files) made by
-- `hp backup`: when each ran and how it went, shown in Settings > System.
-- Not company data: one row per backup of the whole server.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE backup_run (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name        text NOT NULL,
  started_at  timestamptz NOT NULL,
  finished_at timestamptz,
  status      text NOT NULL CHECK (status IN ('running', 'done', 'failed')),
  size_bytes  bigint,
  error       text
);
CREATE INDEX backup_run_started_idx ON backup_run (started_at DESC);
-- The API only reads it; `hp backup` (the admin role) writes it.
REVOKE INSERT, UPDATE, DELETE ON backup_run FROM hiringplug_app;

-- +goose Down
DROP TABLE IF EXISTS backup_run;
