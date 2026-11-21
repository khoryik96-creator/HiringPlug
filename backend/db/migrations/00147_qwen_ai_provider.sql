-- aiproviders: Qwen (Alibaba Cloud Model Studio) joins DeepSeek, Claude and
-- ChatGPT as a provider a company can enter its own key for.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_provider_check,
  ADD CONSTRAINT ai_provider_provider_check CHECK (provider IN ('deepseek', 'claude', 'openai', 'qwen'));
ALTER TABLE ai_provider_choice DROP CONSTRAINT ai_provider_choice_provider_check,
  ADD CONSTRAINT ai_provider_choice_provider_check CHECK (provider IN ('server', 'off', 'deepseek', 'claude', 'openai', 'qwen'));

-- +goose Down
DELETE FROM ai_provider_choice WHERE provider = 'qwen';
DELETE FROM ai_provider WHERE provider = 'qwen';
ALTER TABLE ai_provider_choice DROP CONSTRAINT ai_provider_choice_provider_check,
  ADD CONSTRAINT ai_provider_choice_provider_check CHECK (provider IN ('server', 'off', 'deepseek', 'claude', 'openai'));
ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_provider_check,
  ADD CONSTRAINT ai_provider_provider_check CHECK (provider IN ('deepseek', 'claude', 'openai'));
