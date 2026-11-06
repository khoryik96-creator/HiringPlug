-- aiproviders: the AI provider each company uses, with its own API keys.
-- ai_provider: a provider's model and sealed (encrypted) key; the API never
--   reads the key back, it only passes it on to the AI worker.
-- ai_provider_choice: the provider in use (no row = the server's own).
-- identity: owner and admin roles a company has customised get the new
--   aiproviders:manage, as they would by default.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE ai_provider (
  tenant_id     text NOT NULL REFERENCES tenant (id),
  provider      text NOT NULL CHECK (provider IN ('deepseek', 'claude', 'openai')),
  model         text NOT NULL CHECK (length(model) BETWEEN 1 AND 100),
  sealed_key    text NOT NULL,
  key_hint      text NOT NULL CHECK (length(key_hint) <= 8),
  check_status  text CHECK (check_status IN ('queued', 'ok', 'failed')),
  check_message text CHECK (length(check_message) <= 500),
  checked_at    timestamptz,
  updated_by    text NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, provider)
);
CREATE TABLE ai_provider_choice (
  tenant_id  text PRIMARY KEY REFERENCES tenant (id),
  provider   text NOT NULL CHECK (provider IN ('server', 'off', 'deepseek', 'claude', 'openai')),
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE ai_provider ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_provider
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE ai_provider_choice ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_provider_choice
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

UPDATE role_setting SET permissions = array_append(permissions, 'aiproviders:manage')
WHERE role IN ('owner', 'admin') AND NOT ('aiproviders:manage' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'aiproviders:manage');
DROP TABLE IF EXISTS ai_provider_choice;
DROP TABLE IF EXISTS ai_provider;
