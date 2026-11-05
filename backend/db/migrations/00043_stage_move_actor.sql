-- pipeline: who moved an application (for the dashboard's timeline of stage
-- moves). Null when the app moved it (e.g. an accepted offer, a worker) and
-- for moves recorded before this.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE application_stage_change ADD COLUMN changed_by text CHECK (length(changed_by) <= 40);
CREATE INDEX application_stage_change_recent_idx ON application_stage_change (tenant_id, changed_at DESC);

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

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION application_stage_history() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage)
    VALUES (NEW.tenant_id, NEW.id, NULL, NEW.stage);
  ELSIF NEW.stage IS DISTINCT FROM OLD.stage THEN
    INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage)
    VALUES (NEW.tenant_id, NEW.id, OLD.stage, NEW.stage);
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
DROP INDEX IF EXISTS application_stage_change_recent_idx;
ALTER TABLE application_stage_change DROP COLUMN IF EXISTS changed_by;
