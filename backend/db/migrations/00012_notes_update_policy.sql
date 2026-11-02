-- notes: allow updates (moving notes when duplicate candidates are merged).
-- Without an UPDATE policy, row-level security silently updates nothing.
-- Sensitive notes stay limited to users with notes.sensitive:read.

-- +goose Up
CREATE POLICY note_update ON note FOR UPDATE
  USING (tenant_id = app_current_tenant() AND (NOT sensitive OR app_has_permission('notes.sensitive:read')))
  WITH CHECK (tenant_id = app_current_tenant() AND (NOT sensitive OR app_has_permission('notes.sensitive:read')));

-- +goose Down
DROP POLICY IF EXISTS note_update ON note;
