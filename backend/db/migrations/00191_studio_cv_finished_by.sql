-- "CV formatted" notifications: a user's finished CV Studio runs (the bell
-- asks every minute), and the new kind among those a user can turn off.

-- +goose Up
CREATE INDEX studio_cv_finished_by_idx ON studio_cv (tenant_id, created_by, finished_at DESC) WHERE status IN ('done', 'failed');
ALTER TABLE notification_state DROP CONSTRAINT notification_state_muted_kinds_check;
ALTER TABLE notification_state ADD CONSTRAINT notification_state_muted_kinds_check
  CHECK (muted_kinds <@ ARRAY['applicant', 'task', 'offer_approval', 'interview', 'cv_formatted']);

-- +goose Down
ALTER TABLE notification_state DROP CONSTRAINT notification_state_muted_kinds_check;
ALTER TABLE notification_state ADD CONSTRAINT notification_state_muted_kinds_check
  CHECK (muted_kinds <@ ARRAY['applicant', 'task', 'offer_approval', 'interview']);
DROP INDEX studio_cv_finished_by_idx;
