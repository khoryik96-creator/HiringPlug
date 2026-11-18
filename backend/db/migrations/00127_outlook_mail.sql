-- +goose Up
CREATE TABLE email_outlook_sync (
 mailbox_id text PRIMARY KEY REFERENCES email_mailbox(id) ON DELETE CASCADE,
 tenant_id text NOT NULL DEFAULT app_current_tenant(),
 user_id text NOT NULL DEFAULT app_current_user(),
 generation text NOT NULL,
 requested_at timestamptz NOT NULL DEFAULT now(),
 last_sync_at timestamptz,
 folders_at timestamptz,
 sync_error text
);
CREATE TABLE email_outlook_folder (
 id text PRIMARY KEY DEFAULT new_id('omf'),
 mailbox_id text NOT NULL REFERENCES email_outlook_sync(mailbox_id) ON DELETE CASCADE,
 tenant_id text NOT NULL DEFAULT app_current_tenant(),
 user_id text NOT NULL DEFAULT app_current_user(),
 remote_id text NOT NULL,
 name text NOT NULL,
 kind text NOT NULL,
 parent_id text,
 cursor_url text,
 complete boolean NOT NULL DEFAULT false,
 checked_at timestamptz,
 UNIQUE(mailbox_id,remote_id)
);
CREATE TABLE email_outlook_message (
 id text PRIMARY KEY DEFAULT new_id('omg'),
 mailbox_id text NOT NULL REFERENCES email_outlook_sync(mailbox_id) ON DELETE CASCADE,
 tenant_id text NOT NULL DEFAULT app_current_tenant(),
 user_id text NOT NULL DEFAULT app_current_user(),
 folder_id text NOT NULL REFERENCES email_outlook_folder(id) ON DELETE CASCADE,
 remote_id text NOT NULL,
 conversation text NOT NULL,
 subject text NOT NULL,
 sender text NOT NULL,
 participants text[] NOT NULL,
 received_at timestamptz NOT NULL,
 changed_at timestamptz NOT NULL,
 is_read boolean NOT NULL,
 is_draft boolean NOT NULL,
 has_attachments boolean NOT NULL,
 web_url text,
 UNIQUE(mailbox_id,remote_id)
);
CREATE INDEX email_outlook_recent ON email_outlook_message(mailbox_id,folder_id,received_at DESC,id DESC);
CREATE INDEX email_outlook_thread ON email_outlook_message(mailbox_id,conversation,received_at,id);
CREATE TABLE email_outlook_link (
 mailbox_id text NOT NULL REFERENCES email_outlook_sync(mailbox_id) ON DELETE CASCADE,
 tenant_id text NOT NULL DEFAULT app_current_tenant(),
 user_id text NOT NULL DEFAULT app_current_user(),
 conversation text NOT NULL,
 candidate_id text REFERENCES candidate(id) ON DELETE CASCADE,
 contact_id text REFERENCES contact(id) ON DELETE CASCADE,
 PRIMARY KEY(mailbox_id,conversation),
 CHECK (num_nonnulls(candidate_id,contact_id)=1)
);
CREATE INDEX email_outlook_candidate ON email_outlook_link(tenant_id,candidate_id);
CREATE INDEX email_outlook_contact ON email_outlook_link(tenant_id,contact_id);

ALTER TABLE email_outlook_sync ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_outlook_sync ON email_outlook_sync
 USING (tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('emails:send'));
ALTER TABLE email_outlook_folder ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_outlook_folder ON email_outlook_folder
 USING (tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('emails:send'));
ALTER TABLE email_outlook_message ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_outlook_message ON email_outlook_message
 USING (tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('emails:send'));
ALTER TABLE email_outlook_link ENABLE ROW LEVEL SECURITY;
CREATE POLICY own_outlook_link ON email_outlook_link
 USING (tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('emails:send'));

-- +goose Down
DROP TABLE email_outlook_link,email_outlook_message,email_outlook_folder,email_outlook_sync;
