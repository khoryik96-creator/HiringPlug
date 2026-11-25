-- AI (1.47.0): the server's paid-AI switch. Turning the server's own
-- provider off (HP_AI_PROVIDER=off) never stopped the keys companies saved
-- in Settings > AI provider. This pauses every paid AI call on the server
-- at once: the company keys, the server's own provider, and jobs already
-- queued; local work (text and OCR, PDF copies, search by meaning, reading
-- CVs by the rules) goes on. One row for the server, like backup_control:
-- the API changes it (the agency that runs the server); the AI worker reads
-- it through ai_paid_paused(), as it reads nothing else. A restore from
-- another server turns it on (hp restore; hp paid-ai turns it off again).
-- A key whose check could not run because paid AI was paused is "paused"
-- (not checked), never "failed": the key may well work.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE ai_paid_control (
  id          boolean PRIMARY KEY DEFAULT true CHECK (id),
  paused      boolean NOT NULL DEFAULT false,
  reason      text CHECK (length(reason) <= 300),
  changed_at  timestamptz,
  changed_by  text  -- a user id, or 'hp restore'
);
INSERT INTO ai_paid_control DEFAULT VALUES;
REVOKE INSERT, DELETE ON ai_paid_control FROM hiringplug_app;

-- +goose StatementBegin
CREATE FUNCTION ai_paid_paused() RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT coalesce((SELECT paused FROM ai_paid_control), false)
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION ai_paid_paused() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ai_paid_paused() TO hiringplug_app, hiringplug_ai;

ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_check_status_check;
ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_check_status_check
  CHECK (check_status IN ('queued', 'ok', 'failed', 'paused'));

-- +goose Down
UPDATE ai_provider SET check_status = NULL, check_message = NULL WHERE check_status = 'paused';
ALTER TABLE ai_provider DROP CONSTRAINT ai_provider_check_status_check;
ALTER TABLE ai_provider ADD CONSTRAINT ai_provider_check_status_check
  CHECK (check_status IN ('queued', 'ok', 'failed'));
DROP FUNCTION IF EXISTS ai_paid_paused();
DROP TABLE IF EXISTS ai_paid_control;
