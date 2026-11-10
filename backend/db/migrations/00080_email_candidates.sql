-- emails: an email to a client about several candidates (a shortlist) is
-- kept on each of their emails, in the order it lists them. The email's own
-- candidate_id is the first of them.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE email_candidate (
  tenant_id      text NOT NULL REFERENCES tenant (id),
  email_id       text NOT NULL REFERENCES email (id) ON DELETE CASCADE,
  candidate_id   text NOT NULL,
  -- As written, without a key (like email.application_id): the record survives the application.
  application_id text,
  position       smallint NOT NULL CHECK (position BETWEEN 1 AND 10),
  PRIMARY KEY (email_id, candidate_id),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX email_candidate_candidate_idx ON email_candidate (tenant_id, candidate_id, email_id DESC);

ALTER TABLE email_candidate ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_candidate
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS email_candidate;
