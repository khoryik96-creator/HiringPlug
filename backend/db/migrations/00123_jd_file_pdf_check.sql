-- jobs module: a job description's PDF copy has a file exactly when it is
-- ready, as for documents (00030). Committed migrations are never edited (CI
-- enforces this): add a new file.

-- +goose Up
ALTER TABLE job_description_file
  ADD CONSTRAINT job_description_file_pdf_key_ready CHECK ((pdf_status = 'ready') = (pdf_key IS NOT NULL));

-- +goose Down
ALTER TABLE job_description_file DROP CONSTRAINT IF EXISTS job_description_file_pdf_key_ready;
