-- companies module: clients, prospects and (in-house) your own departments.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE company (
  id                  text PRIMARY KEY DEFAULT new_id('comp') CHECK (id ~ '^comp_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id           text NOT NULL REFERENCES tenant (id),
  version             integer NOT NULL DEFAULT 1,
  name                text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
  type                text NOT NULL DEFAULT 'client' CHECK (type IN ('client', 'prospect', 'internal')),
  website             text CHECK (website ~ '^https?://' AND length(website) <= 300),
  industry            text CHECK (length(industry) <= 100),
  size                text CHECK (size IN ('micro', 'small', 'medium', 'large', 'enterprise')),
  phone               text CHECK (length(phone) <= 40),
  address             text CHECK (length(address) <= 500),
  city                text CHECK (length(city) <= 100),
  country             text CHECK (country ~ '^[A-Z]{2}$'),
  registration_number text CHECK (length(registration_number) <= 50),
  about               text CHECK (length(about) <= 4000),
  -- Name and website, lower-cased and without accents, for search.
  search_text         text GENERATED ALWAYS AS (app_normalize_text(name || ' ' || coalesce(website, ''))) STORED,
  name_key            text GENERATED ALWAYS AS (app_normalize_text(name)) STORED,
  created_by          text NOT NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
-- One company per name (ignoring case and accents) within a tenant.
CREATE UNIQUE INDEX company_tenant_name_uq ON company (tenant_id, name_key);
CREATE INDEX company_tenant_id_idx ON company (tenant_id, id DESC);
CREATE INDEX company_search_trgm_idx ON company USING gin (search_text gin_trgm_ops);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE company ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON company
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS company;
