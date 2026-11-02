-- emails module: templates, and every email sent from the app.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE email_template (
  id         text PRIMARY KEY DEFAULT new_id('etpl') CHECK (id ~ '^etpl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  version    integer NOT NULL DEFAULT 1,
  name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  subject    text NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
  body       text NOT NULL CHECK (length(body) BETWEEN 1 AND 20000),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX email_template_name_idx ON email_template (tenant_id, lower(name));

ALTER TABLE email_template ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_template
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE email (
  id             text PRIMARY KEY DEFAULT new_id('eml') CHECK (id ~ '^eml_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  -- Kept with the candidate (a merge moves them); the application id is kept as
  -- written, without a key, so the record survives the application.
  candidate_id   text,
  application_id text,
  to_addresses   text[] NOT NULL CHECK (cardinality(to_addresses) BETWEEN 1 AND 10),
  cc_addresses   text[] NOT NULL DEFAULT '{}' CHECK (cardinality(cc_addresses) <= 10),
  subject        text NOT NULL CHECK (length(subject) BETWEEN 1 AND 200 AND subject !~ '[\r\n]'),
  body           text NOT NULL CHECK (length(body) BETWEEN 1 AND 20000),
  -- Who sent it: shown as the sender's name, and replies go to their address.
  sent_by        text NOT NULL,
  sender_name    text NOT NULL,
  reply_to       text NOT NULL,
  status         text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'failed')),
  error          text CHECK (length(error) <= 1000),
  created_at     timestamptz NOT NULL DEFAULT now(),
  sent_at        timestamptz,
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX email_candidate_idx ON email (tenant_id, candidate_id, id DESC);

ALTER TABLE email ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS email;
DROP TABLE IF EXISTS email_template;
