-- candidates: their LinkedIn profile address (read from CVs or entered).
-- intake: imports of CVs and LinkedIn PDFs (one or many at once). Each file
--   becomes a candidate named after the file, read like any CV, then filled
--   from it, or merged into the candidate who already has its email.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate ADD COLUMN linkedin_url text CHECK (linkedin_url ~ '^https://');

CREATE TABLE cv_import (
  id         text PRIMARY KEY DEFAULT new_id('cvi') CHECK (id ~ '^cvi_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  source     text NOT NULL CHECK (source ~ '^[a-z0-9_]{1,40}$'),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cv_import_user_idx ON cv_import (tenant_id, created_by, created_at DESC);
CREATE UNIQUE INDEX cv_import_tenant_id_uq ON cv_import (tenant_id, id); -- for the items' foreign key

CREATE TABLE cv_import_item (
  id           text PRIMARY KEY DEFAULT new_id('cvf') CHECK (id ~ '^cvf_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  import_id    text NOT NULL,
  file_name    text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  document_id  text,
  candidate_id text,
  status       text NOT NULL CHECK (status IN ('reading', 'created', 'added', 'unread', 'failed')),
  error        text CHECK (length(error) <= 500),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, import_id) REFERENCES cv_import (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cv_import_item_import_idx ON cv_import_item (tenant_id, import_id);
CREATE INDEX cv_import_item_document_idx ON cv_import_item (tenant_id, document_id);

ALTER TABLE cv_import ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cv_import
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE cv_import_item ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cv_import_item
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS cv_import_item;
DROP TABLE IF EXISTS cv_import;
ALTER TABLE candidate DROP COLUMN IF EXISTS linkedin_url;
