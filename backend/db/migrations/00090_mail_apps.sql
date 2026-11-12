-- emails: an agency's own Microsoft and Google app details for connecting
-- mailboxes, entered in Settings instead of the server's settings file: the
-- client ID, and the secret sealed with HP_MAIL_KEYS_SECRET (never stored in
-- the clear). Without a row the server's HP_MAIL_* values are used.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE email_mail_app (
  tenant_id     text NOT NULL REFERENCES tenant (id),
  provider      text NOT NULL CHECK (provider IN ('microsoft', 'google')),
  client_id     text NOT NULL CHECK (length(client_id) BETWEEN 1 AND 200),
  sealed_secret text NOT NULL,
  secret_ending text NOT NULL CHECK (length(secret_ending) <= 4),
  updated_by    text NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, provider)
);
ALTER TABLE email_mail_app ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON email_mail_app
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS email_mail_app;
