-- pipeline: custom stages. A company saves stage templates (named lists of
--   stages, each of one standard kind: applied ... placed, which reports,
--   automations, offers and placements go by); one may be the default. A job
--   follows the default, or keeps its own copy (from a template, or changed
--   for it). An application remembers which of its job's stages it is in.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE stage_template (
  id          text PRIMARY KEY DEFAULT new_id('stpl') CHECK (id ~ '^stpl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  version     integer NOT NULL DEFAULT 1,
  name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  is_default  boolean NOT NULL DEFAULT false,
  steps       jsonb NOT NULL CHECK (jsonb_typeof(steps) = 'array'),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX stage_template_name_uq ON stage_template (tenant_id, lower(name));
CREATE UNIQUE INDEX stage_template_default_uq ON stage_template (tenant_id) WHERE is_default;

CREATE TABLE job_stage_plan (
  tenant_id    text NOT NULL REFERENCES tenant (id),
  job_order_id text NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  template_id  text,
  steps        jsonb NOT NULL CHECK (jsonb_typeof(steps) = 'array'),
  customized   boolean NOT NULL DEFAULT false,
  updated_by   text NOT NULL,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, job_order_id),
  FOREIGN KEY (tenant_id, job_order_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX job_stage_plan_template_idx ON job_stage_plan (tenant_id, template_id);

ALTER TABLE application ADD COLUMN step text CHECK (step ~ '^[a-z0-9_]{1,40}$');

ALTER TABLE stage_template ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON stage_template
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE job_stage_plan ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON job_stage_plan
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
ALTER TABLE application DROP COLUMN IF EXISTS step;
DROP TABLE IF EXISTS job_stage_plan;
DROP TABLE IF EXISTS stage_template;
