-- assistant module: questions to the AI assistant and their answers.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE assistant_answer (
  id            text PRIMARY KEY DEFAULT new_id('ask') CHECK (id ~ '^ask_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  user_id       text NOT NULL,
  subject_type  text NOT NULL CHECK (subject_type IN ('candidate', 'job', 'application')),
  subject_id    text NOT NULL CHECK (length(subject_id) <= 40),
  kind          text NOT NULL CHECK (kind IN ('summary', 'red_flags', 'fit', 'question')),
  question      text CHECK (length(question) <= 1000),
  status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  answer        text CHECK (length(answer) <= 20000),
  error         text CHECK (length(error) <= 1000),
  model         text CHECK (length(model) <= 200),
  input_tokens  integer NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens integer NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  created_at    timestamptz NOT NULL DEFAULT now(),
  answered_at   timestamptz,
  CHECK ((kind = 'question') = (question IS NOT NULL))
);
CREATE INDEX assistant_answer_subject_idx ON assistant_answer (tenant_id, user_id, subject_type, subject_id, created_at DESC);
CREATE INDEX assistant_answer_month_idx ON assistant_answer (tenant_id, created_at);

ALTER TABLE assistant_answer ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assistant_answer
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS assistant_answer;
