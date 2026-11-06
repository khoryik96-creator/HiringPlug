-- contact_employment_check: where a contact works now, read from their
-- LinkedIn profile saved as PDF. The PDF is kept only until it is read;
-- what stays is the jobs it lists and how they compare with the contact's
-- company.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE contact_employment_check (
  id           text PRIMARY KEY DEFAULT new_id('cec') CHECK (id ~ '^cec_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  contact_id   text NOT NULL,
  file_name    text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  storage_key  text,
  status       text NOT NULL DEFAULT 'reading' CHECK (status IN ('reading', 'read', 'failed')),
  error        text CHECK (length(error) <= 500),
  result       jsonb,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, contact_id) REFERENCES contact (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX contact_employment_check_contact_idx ON contact_employment_check (tenant_id, contact_id, created_at DESC);

ALTER TABLE contact_employment_check ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON contact_employment_check
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS contact_employment_check;
