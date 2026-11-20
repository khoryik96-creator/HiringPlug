-- emails module: each person's email signature (formatted, with banners),
-- added to the emails they write in Outlook Mail.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE email_signature (
  tenant_id  text NOT NULL REFERENCES tenant (id),
  user_id    text NOT NULL,
  body_html  text NOT NULL CHECK (length(body_html) <= 40000),
  version    integer NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id),
  FOREIGN KEY (tenant_id, user_id) REFERENCES app_user (tenant_id, id) ON DELETE CASCADE
);

-- A signature is its owner's alone.
ALTER TABLE email_signature ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_signature AS RESTRICTIVE
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY email_signature_own ON email_signature
  USING (user_id = app_current_user()) WITH CHECK (user_id = app_current_user());

-- +goose Down
DROP TABLE IF EXISTS email_signature;
