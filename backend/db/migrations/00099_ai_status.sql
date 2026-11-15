-- Whether the AI provider a company uses is answering: the AI worker
-- reports each job's last call (it worked, or why not), so the app can tell
-- people when AI features are not working instead of leaving them waiting.
-- One row per company: the latest answer and the latest failure.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE ai_provider_status (
  tenant_id     text PRIMARY KEY REFERENCES tenant (id),
  provider      text NOT NULL CHECK (length(provider) BETWEEN 1 AND 40),
  last_ok_at    timestamptz,
  failed_at     timestamptz,
  failing_since timestamptz,
  error         text CHECK (length(error) <= 500)
);
ALTER TABLE ai_provider_status ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_provider_status
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS ai_provider_status;
