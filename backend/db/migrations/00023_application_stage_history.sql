-- pipeline module: every stage an application has been at, and when, for
-- reports (how far candidates get, how long placing takes). A trigger
-- records each change, whichever code path makes it.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE application_stage_change (
  id             text PRIMARY KEY DEFAULT new_id('stg') CHECK (id ~ '^stg_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL,
  application_id text NOT NULL,
  from_stage     text, -- null when the application was created at to_stage
  to_stage       text NOT NULL,
  changed_at     timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, application_id) REFERENCES application (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX application_stage_change_idx ON application_stage_change (tenant_id, application_id, changed_at);
CREATE INDEX application_stage_change_to_idx ON application_stage_change (tenant_id, to_stage, changed_at);

ALTER TABLE application_stage_change ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON application_stage_change
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose StatementBegin
CREATE FUNCTION application_stage_history() RETURNS trigger
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
CREATE TRIGGER application_stage_history AFTER INSERT OR UPDATE OF stage ON application
  FOR EACH ROW EXECUTE FUNCTION application_stage_history();

-- The history so far, from the audit log (created and moved entries), and
-- for applications without one, their current stage since they were created.
INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, changed_at)
SELECT l.tenant_id, l.entity_id, NULL, l.changes->>'stage', l.created_at
FROM audit_log l JOIN application a ON a.tenant_id = l.tenant_id AND a.id = l.entity_id
WHERE l.entity_type = 'application' AND l.action = 'application.created' AND jsonb_typeof(l.changes->'stage') = 'string';

INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, changed_at)
SELECT l.tenant_id, l.entity_id, l.changes->'stage'->>'from', l.changes->'stage'->>'to', l.created_at
FROM audit_log l JOIN application a ON a.tenant_id = l.tenant_id AND a.id = l.entity_id
WHERE l.entity_type = 'application' AND l.action = 'application.moved' AND jsonb_typeof(l.changes->'stage') = 'object'
  AND l.changes->'stage'->>'to' IS NOT NULL;

INSERT INTO application_stage_change (tenant_id, application_id, from_stage, to_stage, changed_at)
SELECT a.tenant_id, a.id, NULL, a.stage, a.created_at
FROM application a
WHERE NOT EXISTS (SELECT 1 FROM application_stage_change c WHERE c.tenant_id = a.tenant_id AND c.application_id = a.id);

-- +goose Down
DROP TRIGGER IF EXISTS application_stage_history ON application;
DROP FUNCTION IF EXISTS application_stage_history();
DROP TABLE IF EXISTS application_stage_change;
