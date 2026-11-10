-- candidates: each screening call kept (screening_call): when it was, who
--   made it, its note, and whether it set the salary, so a candidate's salary
--   follows their latest call and the calls can be listed.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE screening_call (
  id              text PRIMARY KEY DEFAULT new_id('scl') CHECK (id ~ '^scl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id       text NOT NULL,
  candidate_id    text NOT NULL,
  called_on       date NOT NULL,
  called_by       text NOT NULL,
  note_id         text,
  salary_recorded boolean NOT NULL DEFAULT false,
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX screening_call_idx ON screening_call (tenant_id, candidate_id, called_on DESC, created_at DESC);
ALTER TABLE screening_call ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON screening_call
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS screening_call;
