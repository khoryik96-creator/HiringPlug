-- job_description_file: a client's job description uploaded to fill in an
-- ad. Its text is read by the AI worker (PDF, Word) or at once (text), then
-- the ad's details are read from it; nothing on the job or an ad changes
-- until someone saves the ad form.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE job_description_file (
  id           text PRIMARY KEY DEFAULT new_id('jdf') CHECK (id ~ '^jdf_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  job_id       text NOT NULL,
  file_name    text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  content_type text NOT NULL CHECK (length(content_type) BETWEEN 1 AND 255),
  size_bytes   integer NOT NULL CHECK (size_bytes BETWEEN 1 AND 26214400),
  storage_key  text NOT NULL,
  status       text NOT NULL DEFAULT 'reading' CHECK (status IN ('reading', 'read', 'failed')),
  error        text CHECK (length(error) <= 500),
  text         text CHECK (length(text) <= 200000),
  details      jsonb,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, job_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX job_description_file_job_idx ON job_description_file (tenant_id, job_id);

ALTER TABLE job_description_file ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_description_file
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS job_description_file;
