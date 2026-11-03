-- companies module: details suggested from a company's own website.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE company
  ADD COLUMN autofill_status text NOT NULL DEFAULT 'idle' CHECK (autofill_status IN ('idle', 'queued', 'done', 'failed')),
  ADD COLUMN autofill_error text CHECK (length(autofill_error) <= 1000),
  ADD COLUMN autofill_checked_at timestamptz;

CREATE TABLE company_suggestion (
  id         text PRIMARY KEY DEFAULT new_id('csug') CHECK (id ~ '^csug_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL,
  company_id text NOT NULL,
  field      text NOT NULL CHECK (field IN ('name', 'phone', 'address', 'city', 'country', 'registrationNumber', 'size', 'about')),
  value      text NOT NULL CHECK (length(value) BETWEEN 1 AND 4000),
  source     text NOT NULL CHECK (length(source) <= 500),
  status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'dismissed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX company_suggestion_idx ON company_suggestion (tenant_id, company_id, status);

ALTER TABLE company_suggestion ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON company_suggestion
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS company_suggestion;
ALTER TABLE company DROP COLUMN IF EXISTS autofill_status, DROP COLUMN IF EXISTS autofill_error, DROP COLUMN IF EXISTS autofill_checked_at;
