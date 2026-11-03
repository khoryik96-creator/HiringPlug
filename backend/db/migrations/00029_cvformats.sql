-- cvformats module: the company's own Word CV templates.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE cv_template (
  id          text PRIMARY KEY DEFAULT new_id('ctpl') CHECK (id ~ '^ctpl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  storage_key text NOT NULL,
  uploaded_by text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);

ALTER TABLE cv_template ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cv_template
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS cv_template;
