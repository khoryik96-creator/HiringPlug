-- +goose Up
CREATE TABLE email_mailbox (
 id text NOT NULL DEFAULT new_id('mbx') UNIQUE,
 tenant_id text NOT NULL DEFAULT app_current_tenant() REFERENCES tenant(id),
 user_id text NOT NULL DEFAULT app_current_user(),
 provider text NOT NULL CHECK(provider IN ('smtp','google','microsoft')),
 address text NOT NULL CHECK(length(address) BETWEEN 3 AND 254),
 host text NOT NULL DEFAULT '', port integer NOT NULL DEFAULT 587,
 tls text NOT NULL DEFAULT 'starttls', username text NOT NULL DEFAULT '',
 sealed_secret text NOT NULL, version integer NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,user_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
ALTER TABLE email_mailbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY mailbox_scope ON email_mailbox USING (
 tenant_id=app_current_tenant() AND app_has_permission('emails:send')
 AND (user_id=app_current_user() OR app_current_user()='system')
) WITH CHECK (
 tenant_id=app_current_tenant() AND app_has_permission('emails:send')
 AND (user_id=app_current_user() OR app_current_user()='system')
);
CREATE TABLE email_mailbox_state (
 tenant_id text NOT NULL DEFAULT app_current_tenant() REFERENCES tenant(id),
 user_id text NOT NULL DEFAULT app_current_user(),
 state_hash text NOT NULL UNIQUE, provider text NOT NULL,
 sealed_verifier text NOT NULL, consumed boolean NOT NULL DEFAULT false, expires_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,user_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
ALTER TABLE email_mailbox_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY mailbox_state_scope ON email_mailbox_state USING (
 tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('emails:send')
) WITH CHECK (
 tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('emails:send')
);
-- Keep the selected delivery route, so disconnecting cannot silently send via
-- another account or move queued local test messages to a live provider.
ALTER TABLE email ADD COLUMN mailbox_ref text;
ALTER TABLE email ADD COLUMN delivery_mode text NOT NULL DEFAULT 'unknown' CHECK(delivery_mode IN ('test','live','unknown'));
-- +goose Down
ALTER TABLE email DROP COLUMN delivery_mode,DROP COLUMN mailbox_ref;
DROP TABLE email_mailbox_state;
DROP TABLE email_mailbox;
