-- documents: tokens the AI used reading CVs (filling what the rules left
-- empty), one row per reading, counted in the company's monthly allowance
-- with the assistant's and matching's.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE document_ai_use (
  tenant_id   text NOT NULL REFERENCES tenant (id),
  document_id text NOT NULL,
  tokens      integer NOT NULL CHECK (tokens >= 0),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX document_ai_use_month_idx ON document_ai_use (tenant_id, created_at);

ALTER TABLE document_ai_use ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON document_ai_use
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS document_ai_use;
