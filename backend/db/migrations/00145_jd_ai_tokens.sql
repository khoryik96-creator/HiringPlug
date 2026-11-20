-- The AI tokens used reading an uploaded job description (the AI reads its
-- details when the company has an AI provider): counted in the company's
-- monthly AI allowance.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_description_file ADD COLUMN ai_tokens integer NOT NULL DEFAULT 0 CHECK (ai_tokens >= 0);
CREATE INDEX job_description_file_ai_month_idx ON job_description_file (tenant_id, created_at) WHERE ai_tokens > 0;

-- +goose Down
DROP INDEX IF EXISTS job_description_file_ai_month_idx;
ALTER TABLE job_description_file DROP COLUMN IF EXISTS ai_tokens;
