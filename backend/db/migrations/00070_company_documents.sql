-- companies: documents kept with a client's terms of business or benefits
--   (the signed terms, a benefits brochure), their files in blob storage.
-- identity: admins who chose their own permissions get the new
--   companies.documents:delete (owners have every permission).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE company_document (
  id           text PRIMARY KEY DEFAULT new_id('cdoc') CHECK (id ~ '^cdoc_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL,
  company_id   text NOT NULL,
  section      text NOT NULL CHECK (section IN ('terms', 'benefits')),
  file_name    text NOT NULL CHECK (length(file_name) BETWEEN 1 AND 255),
  content_type text NOT NULL CHECK (length(content_type) BETWEEN 1 AND 255),
  size_bytes   integer NOT NULL CHECK (size_bytes > 0),
  storage_key  text NOT NULL,
  created_by   text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX company_document_idx ON company_document (tenant_id, company_id, section, created_at);
ALTER TABLE company_document ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON company_document
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

UPDATE role_setting SET permissions = array_append(permissions, 'companies.documents:delete')
WHERE role = 'admin' AND NOT ('companies.documents:delete' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'companies.documents:delete');
DROP TABLE IF EXISTS company_document;
