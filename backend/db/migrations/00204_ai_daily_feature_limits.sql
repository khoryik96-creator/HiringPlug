-- aiusage: a company's daily AI limit (all features together) and each
-- feature's own monthly limit, in tokens, besides the month's limit.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE ai_usage_setting
  ADD COLUMN daily_tokens bigint CHECK (daily_tokens > 0),
  ADD COLUMN feature_tokens jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(feature_tokens) = 'object');

-- +goose Down
ALTER TABLE ai_usage_setting DROP COLUMN feature_tokens, DROP COLUMN daily_tokens;
