-- Signing out ends the session on the server too: the token's ID is kept
-- here until the token would have expired anyway, and is refused meanwhile
-- (a copied token stops working at sign-out, not hours later).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE revoked_session (
  id         text PRIMARY KEY CHECK (length(id) BETWEEN 10 AND 64),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  user_id    text NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX revoked_session_expires_idx ON revoked_session (expires_at);
ALTER TABLE revoked_session ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON revoked_session
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS revoked_session;
