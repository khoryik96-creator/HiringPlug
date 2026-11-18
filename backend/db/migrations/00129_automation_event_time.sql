-- automations: an automation.event says when the change happened, so a run
--   "2 days after" counts from then, not from when the worker got to it (a
--   queue that was behind, a retry).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION automation_fan_out() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.action IN ('application.created', 'application.moved', 'interview.scheduled', 'interview.updated',
                    'placement.recorded', 'placement.updated', 'placement.invoicing_set')
     AND EXISTS (SELECT 1 FROM automation WHERE tenant_id = NEW.tenant_id AND enabled) THEN
    PERFORM job_enqueue(NEW.tenant_id, 'core', 'automation.event', jsonb_build_object(
      'action', NEW.action, 'entityType', NEW.entity_type, 'entityId', NEW.entity_id, 'changes', NEW.changes,
      'occurredAt', to_char(NEW.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')), NEW.request_id);
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION automation_fan_out() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.action IN ('application.created', 'application.moved', 'interview.scheduled', 'interview.updated',
                    'placement.recorded', 'placement.updated', 'placement.invoicing_set')
     AND EXISTS (SELECT 1 FROM automation WHERE tenant_id = NEW.tenant_id AND enabled) THEN
    PERFORM job_enqueue(NEW.tenant_id, 'core', 'automation.event', jsonb_build_object(
      'action', NEW.action, 'entityType', NEW.entity_type, 'entityId', NEW.entity_id, 'changes', NEW.changes), NEW.request_id);
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
