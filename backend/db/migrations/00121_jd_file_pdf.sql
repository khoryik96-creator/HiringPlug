-- jobs module: a Word (or RTF, ODT) job description also gets a PDF copy, so
-- Job details and the job's ads preview it as it looks instead of as text.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE job_description_file
  ADD COLUMN pdf_status text CHECK (pdf_status IN ('queued', 'ready', 'failed')),
  ADD COLUMN pdf_key text,
  ADD COLUMN pdf_error text CHECK (length(pdf_error) <= 1000);

-- +goose Down
ALTER TABLE job_description_file DROP COLUMN IF EXISTS pdf_error, DROP COLUMN IF EXISTS pdf_key, DROP COLUMN IF EXISTS pdf_status;
