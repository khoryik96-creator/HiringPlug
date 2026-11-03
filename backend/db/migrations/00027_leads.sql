-- leads module: companies that are hiring, as business development leads,
-- and the public sources they are collected from.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE lead_source (
  id              text PRIMARY KEY DEFAULT new_id('lsrc') CHECK (id ~ '^lsrc_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id       text NOT NULL REFERENCES tenant (id),
  version         integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  kind            text NOT NULL CHECK (kind IN ('greenhouse', 'lever', 'ashby', 'smartrecruiters', 'careers_page')),
  identifier      text NOT NULL CHECK (length(identifier) BETWEEN 1 AND 500),
  name            text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
  company_id      text, -- checked by the service through the companies module
  active          boolean NOT NULL DEFAULT true,
  open_leads      integer NOT NULL DEFAULT 0,
  last_checked_at timestamptz,
  last_error      text CHECK (length(last_error) <= 1000),
  next_check_at   timestamptz,
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, kind, identifier)
);

ALTER TABLE lead_source ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON lead_source
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE job_lead (
  id            text PRIMARY KEY DEFAULT new_id('lead') CHECK (id ~ '^lead_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  version       integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  -- Kept when the source is removed (the lead is still worth having).
  source_id     text,
  source_kind   text,
  external_id   text CHECK (length(external_id) <= 300), -- the opening's id at the source
  company_name  text NOT NULL CHECK (length(btrim(company_name)) BETWEEN 1 AND 200),
  company_id    text,
  title         text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  location      text CHECK (length(location) <= 200),
  url           text CHECK (length(url) <= 1000),
  posted_at     timestamptz,
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  closed_at     timestamptz,
  status        text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'contacted', 'client', 'ignored')),
  notes         text CHECK (length(notes) <= 2000),
  created_by    text NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, source_id) REFERENCES lead_source (tenant_id, id) ON DELETE SET NULL (source_id),
  CHECK ((source_id IS NULL) OR (external_id IS NOT NULL))
);
-- One lead per opening at a source.
CREATE UNIQUE INDEX job_lead_opening_idx ON job_lead (tenant_id, source_id, external_id) WHERE source_id IS NOT NULL;
CREATE INDEX job_lead_list_idx ON job_lead (tenant_id, id DESC);

ALTER TABLE job_lead ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_lead
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS job_lead;
DROP TABLE IF EXISTS lead_source;
