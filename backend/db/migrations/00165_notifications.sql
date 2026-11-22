-- notifications module: the bell in the top bar and the morning email.
-- The notifications themselves are not stored: they are worked out from the
-- records when asked (new applicants on your jobs, your tasks due, offers
-- waiting for your approval, your interviews today and tomorrow). This table
-- keeps, per user, when they last looked (what is new since) and whether
-- they want the morning email.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE notification_state (
  tenant_id     text NOT NULL REFERENCES tenant (id),
  user_id       text NOT NULL,
  seen_at       timestamptz,
  morning_email boolean NOT NULL DEFAULT false,
  -- The kinds the user turned off (Settings > Your account): they show
  -- neither in the bell nor in the morning email.
  muted_kinds   text[] NOT NULL DEFAULT '{}'
                CHECK (muted_kinds <@ ARRAY['applicant', 'task', 'offer_approval', 'interview']),
  -- Each time the morning email is turned on a new daily chain of jobs
  -- starts; a job from an older chain (turned off and on again) does nothing.
  digest_chain  integer NOT NULL DEFAULT 0 CHECK (digest_chain >= 0),
  -- The agency-time day of the last morning email: a retried job never
  -- sends the same day's email twice.
  emailed_on    date,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);

-- Every table holding tenant data has row-level security (a test checks
-- this). Each user reads and changes only their own row; the system (the
-- morning email's job) reads any.
ALTER TABLE notification_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON notification_state
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY own_row ON notification_state AS RESTRICTIVE
  USING (user_id = app_current_user() OR app_current_user() = 'system')
  WITH CHECK (user_id = app_current_user() OR app_current_user() = 'system');

-- +goose Down
DROP TABLE IF EXISTS notification_state;
