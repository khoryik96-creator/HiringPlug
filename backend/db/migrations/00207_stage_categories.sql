-- pipeline (1.57.0): stage categories. Every stage of a template or a job
-- falls under a category, which reports count by (the funnel, a candidate's
-- tag). Until now the six standard kinds (applied ... placed) were the only
-- categories, so every interview round was "Interview". A company now keeps
-- its own list: rename one, add more (1st, 2nd and Final Interview), order
-- them, remove an unused one. Each category counts as one of the six kinds,
-- which automations, offers and placements still go by; that kind never
-- changes once the category exists. A company with no rows has the six
-- standard categories (keys = the kinds); a stage or application with no
-- category is in its kind's standard category.
-- The stage history now records category moves too (1st to 2nd Interview,
-- the same kind), so reports can count who reached each category.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE stage_category (
  tenant_id   text NOT NULL REFERENCES tenant (id),
  key         text NOT NULL CHECK (key ~ '^[a-z0-9_]{1,40}$'),
  label       text NOT NULL CHECK (length(label) BETWEEN 1 AND 40),
  kind        text NOT NULL CHECK (kind IN ('applied', 'screening', 'submitted', 'interview', 'offer', 'placed')),
  position    integer NOT NULL CHECK (position BETWEEN 0 AND 99),
  updated_by  text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, key)
);
CREATE UNIQUE INDEX stage_category_label_uq ON stage_category (tenant_id, lower(label));

ALTER TABLE stage_category ENABLE ROW LEVEL SECURITY;
-- Everyone in the company reads them (boards, reports); only
-- pipeline.categories:manage changes them.
CREATE POLICY tenant_read ON stage_category FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY manage_insert ON stage_category FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('pipeline.categories:manage'));
CREATE POLICY manage_update ON stage_category FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('pipeline.categories:manage'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('pipeline.categories:manage'));
CREATE POLICY manage_delete ON stage_category FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('pipeline.categories:manage'));

-- The category an application's stage falls under (null: its kind's standard one).
ALTER TABLE application ADD COLUMN category text CHECK (category ~ '^[a-z0-9_]{1,40}$');
ALTER TABLE application_stage_change ADD COLUMN from_category text, ADD COLUMN to_category text;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION application_stage_history() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, to_category, changed_by)
    VALUES (NEW.tenant_id, NEW.id, NULL, NEW.stage, NEW.category, app_current_user());
  ELSIF NEW.stage IS DISTINCT FROM OLD.stage OR NEW.category IS DISTINCT FROM OLD.category THEN
    INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, from_category, to_category, changed_by)
    VALUES (NEW.tenant_id, NEW.id, OLD.stage, NEW.stage, OLD.category, NEW.category, app_current_user());
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION application_stage_history() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, changed_by)
    VALUES (NEW.tenant_id, NEW.id, NULL, NEW.stage, app_current_user());
  ELSIF NEW.stage IS DISTINCT FROM OLD.stage THEN
    INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, changed_by)
    VALUES (NEW.tenant_id, NEW.id, OLD.stage, NEW.stage, app_current_user());
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
ALTER TABLE application_stage_change DROP COLUMN IF EXISTS from_category, DROP COLUMN IF EXISTS to_category;
ALTER TABLE application DROP COLUMN IF EXISTS category;
DROP TABLE IF EXISTS stage_category;
