-- Audit trail: who changed what, when, and from which request.
-- Written in the same transaction as the change it describes, so the trail
-- can never miss a committed change.

-- +goose Up
CREATE TABLE audit_log (
  id          text PRIMARY KEY DEFAULT new_id('aud') CHECK (id ~ '^aud_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  actor_id    text NOT NULL,              -- usr_... or 'system' for workers
  action      text NOT NULL CHECK (action ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$'),
  entity_type text NOT NULL,
  entity_id   text NOT NULL,
  -- Changed field names with old/new values. Sensitive fields (salary) record
  -- only that they changed, never the values.
  changes     jsonb NOT NULL DEFAULT '{}'::jsonb,
  request_id  text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_entity_idx ON audit_log (tenant_id, entity_type, entity_id, created_at DESC);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_log
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS audit_log;
