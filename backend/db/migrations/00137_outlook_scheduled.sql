-- +goose Up
-- An independently committed receipt makes each reviewed draft a single
-- dispatch attempt even if the worker loses its lease after Outlook accepts it.
CREATE TABLE email_outlook_send (
  id text PRIMARY KEY DEFAULT new_id('oms'),
  tenant_id text NOT NULL DEFAULT app_current_tenant(),
  user_id text NOT NULL DEFAULT app_current_user(),
  mailbox_id text NOT NULL,
  message_id text NOT NULL,
  remote_id text NOT NULL,
  draft_version text NOT NULL,
  candidate_id text,
  subject text NOT NULL,
  send_at timestamptz NOT NULL,
  version integer NOT NULL DEFAULT 1,
  status text NOT NULL DEFAULT 'scheduled'
    CHECK (status IN ('scheduled','confirming','sent','needs_review','uncertain','cancelled')),
  attempted_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  error text,
  FOREIGN KEY (tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX email_outlook_one_send
 ON email_outlook_send(mailbox_id,remote_id) WHERE status IN ('scheduled','confirming','uncertain');
CREATE INDEX email_outlook_send_user ON email_outlook_send(tenant_id,user_id,updated_at DESC,id);
ALTER TABLE email_outlook_send ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_outlook_send ON email_outlook_send USING (
 tenant_id=app_current_tenant() AND app_has_permission('emails:send')
 AND (user_id=app_current_user() OR app_current_user()='system')
);

-- +goose Down
DROP TABLE email_outlook_send;
