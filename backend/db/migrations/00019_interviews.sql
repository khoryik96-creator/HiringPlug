-- interviews module: interviews for an application, and each interviewer's feedback.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE interview (
  id               text PRIMARY KEY DEFAULT new_id('intv') CHECK (id ~ '^intv_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id        text NOT NULL REFERENCES tenant (id),
  application_id   text NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  title            text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 100),
  starts_at        timestamptz NOT NULL,
  duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 5 AND 480),
  -- starts_at + duration (kept with it by the service; a generated column cannot add intervals to timestamptz).
  ends_at          timestamptz NOT NULL,
  CHECK (ends_at = starts_at + make_interval(mins => duration_minutes)),
  mode             text NOT NULL CHECK (mode IN ('in_person', 'video', 'phone')),
  location         text CHECK (length(location) <= 500),
  instructions     text CHECK (length(instructions) <= 2000),
  status           text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show')),
  -- Users of the same tenant (checked by the service; users are never deleted, only deactivated).
  interviewer_ids  text[] NOT NULL CHECK (cardinality(interviewer_ids) BETWEEN 1 AND 10),
  created_by       text NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, application_id) REFERENCES application (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX interview_application_idx ON interview (tenant_id, application_id, starts_at);
CREATE INDEX interview_time_idx ON interview (tenant_id, starts_at);
CREATE INDEX interview_interviewers_idx ON interview USING gin (interviewer_ids);

ALTER TABLE interview ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON interview
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE interview_feedback (
  tenant_id      text NOT NULL,
  interview_id   text NOT NULL,
  user_id        text NOT NULL,
  recommendation text NOT NULL CHECK (recommendation IN ('strong_yes', 'yes', 'no', 'strong_no')),
  comments       text CHECK (length(comments) <= 5000),
  submitted_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, interview_id, user_id),
  FOREIGN KEY (tenant_id, interview_id) REFERENCES interview (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE interview_feedback ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON interview_feedback
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS interview_feedback;
DROP TABLE IF EXISTS interview;
