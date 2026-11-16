-- Emails the AI writes for someone (write, improve or reply), shown only to
-- them. What it was sent is not kept: only what it wrote, with the
-- placeholders the browser fills back in. Its tokens count towards the
-- company's monthly AI allowance.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE assistant_email (
  id            text PRIMARY KEY DEFAULT new_id('aem') CHECK (id ~ '^aem_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  user_id       text NOT NULL,
  mode          text NOT NULL CHECK (mode IN ('write', 'improve', 'reply')),
  status        text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'done', 'failed')),
  subject       text CHECK (length(subject) <= 1000),
  body          text CHECK (length(body) <= 40000),
  error         text CHECK (length(error) <= 1000),
  model         text CHECK (length(model) <= 200),
  input_tokens  integer NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens integer NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  created_at    timestamptz NOT NULL DEFAULT now(),
  written_at    timestamptz
);
CREATE INDEX assistant_email_month_idx ON assistant_email (tenant_id, created_at);

ALTER TABLE assistant_email ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON assistant_email
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS assistant_email;
