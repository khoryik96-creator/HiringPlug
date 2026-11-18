-- +goose Up
-- Personal calendar data and credentials are private even to agency owners.
CREATE TABLE calendar_connection (
 tenant_id text NOT NULL REFERENCES tenant(id), user_id text NOT NULL,
 id text NOT NULL DEFAULT new_id('ccn') UNIQUE, version integer NOT NULL DEFAULT 1,
 subject_id text NOT NULL, address text NOT NULL, app_client_id text NOT NULL,
 calendar_id text NOT NULL, calendar_name text NOT NULL, teams_available boolean NOT NULL DEFAULT false,
 sealed_tokens text NOT NULL, status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','ok','failed','reconnect','disconnected')),
 sync_sequence integer NOT NULL DEFAULT 1, synced_at timestamptz, sync_error text,
 PRIMARY KEY(tenant_id,user_id), FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX calendar_account_owner ON calendar_connection(tenant_id,subject_id,calendar_id) WHERE status<>'disconnected';
CREATE TABLE calendar_oauth_state (
 tenant_id text NOT NULL REFERENCES tenant(id), user_id text NOT NULL,
 state_hash text NOT NULL UNIQUE, app_client_id text NOT NULL, sealed_verifier text NOT NULL,
 sealed_code text, ticket_hash text, consumed boolean NOT NULL DEFAULT false, expires_at timestamptz NOT NULL,
 PRIMARY KEY(tenant_id,user_id), FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE calendar_event (
 id text PRIMARY KEY DEFAULT new_id('cev') CHECK(id ~ '^cev_[0-9a-hjkmnp-tv-z]{26}$'),
 tenant_id text NOT NULL REFERENCES tenant(id), user_id text NOT NULL, version integer NOT NULL DEFAULT 1,
 title text NOT NULL CHECK(length(title) BETWEEN 1 AND 200), notes text NOT NULL DEFAULT '' CHECK(length(notes)<=4000),
 location text NOT NULL DEFAULT '' CHECK(length(location)<=500), starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),
 time_zone text NOT NULL DEFAULT 'UTC', all_day boolean NOT NULL DEFAULT false, teams boolean NOT NULL DEFAULT false,
 attendees jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(attendees)='array'),
 source text NOT NULL CHECK(source IN ('local','interview','booking','outlook')), source_id text, source_version integer NOT NULL DEFAULT 0,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','cancelled')),
 sync_status text NOT NULL DEFAULT 'local' CHECK(sync_status IN ('local','pending','synced','conflict','failed')), sync_error text,
 remote_generation integer NOT NULL DEFAULT 0, remote_id text, remote_key text, remote_connection text, sent_version integer NOT NULL DEFAULT 0,
 web_url text, join_url text, remote_snapshot jsonb,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(tenant_id,user_id,source,source_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX calendar_event_window ON calendar_event(tenant_id,user_id,starts_at);
CREATE UNIQUE INDEX calendar_event_remote ON calendar_event(tenant_id,user_id,remote_connection,remote_id) WHERE remote_id IS NOT NULL;
-- Only these minimal rows may be shared with schedulers: never titles or notes.
CREATE TABLE outlook_availability (
 tenant_id text NOT NULL REFERENCES tenant(id), user_id text NOT NULL,
 connection_id text NOT NULL, status text NOT NULL, synced_at timestamptz,
 window_start timestamptz, window_end timestamptz,
 PRIMARY KEY(tenant_id,user_id), FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
CREATE TABLE outlook_busy (
 tenant_id text NOT NULL REFERENCES tenant(id), user_id text NOT NULL,
 starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL CHECK(ends_at>starts_at),
 origin text NOT NULL DEFAULT 'outlook', source text NOT NULL DEFAULT 'outlook', source_id text NOT NULL,
 UNIQUE(tenant_id,user_id,origin,source,source_id),
 FOREIGN KEY(tenant_id,user_id) REFERENCES app_user(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX outlook_busy_window ON outlook_busy(tenant_id,user_id,starts_at);
ALTER TABLE calendar_connection ENABLE ROW LEVEL SECURITY;
CREATE POLICY personal_calendar ON calendar_connection USING(tenant_id=app_current_tenant() AND app_has_permission('calendars:use') AND (user_id=app_current_user() OR app_current_user()='system')) WITH CHECK(tenant_id=app_current_tenant() AND app_has_permission('calendars:use') AND (user_id=app_current_user() OR app_current_user()='system'));
ALTER TABLE calendar_oauth_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY personal_state ON calendar_oauth_state USING(tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('calendars:use')) WITH CHECK(tenant_id=app_current_tenant() AND user_id=app_current_user() AND app_has_permission('calendars:use'));
ALTER TABLE calendar_event ENABLE ROW LEVEL SECURITY;
CREATE POLICY personal_events ON calendar_event USING(tenant_id=app_current_tenant() AND app_has_permission('calendars:use') AND (user_id=app_current_user() OR app_current_user()='system')) WITH CHECK(tenant_id=app_current_tenant() AND app_has_permission('calendars:use') AND (user_id=app_current_user() OR app_current_user()='system'));
ALTER TABLE outlook_availability ENABLE ROW LEVEL SECURITY;
CREATE POLICY availability_read ON outlook_availability FOR SELECT USING(tenant_id=app_current_tenant() AND (app_has_permission('calendars:use') OR app_has_permission('scheduling:use') OR app_has_permission('interviews:schedule')));
CREATE POLICY availability_write ON outlook_availability FOR ALL USING(tenant_id=app_current_tenant() AND (user_id=app_current_user() OR app_current_user()='system') AND app_has_permission('calendars:use')) WITH CHECK(tenant_id=app_current_tenant() AND (user_id=app_current_user() OR app_current_user()='system') AND app_has_permission('calendars:use'));
ALTER TABLE outlook_busy ENABLE ROW LEVEL SECURITY;
CREATE POLICY busy_read ON outlook_busy FOR SELECT USING(tenant_id=app_current_tenant() AND (app_has_permission('calendars:use') OR app_has_permission('scheduling:use') OR app_has_permission('interviews:schedule')));
CREATE POLICY busy_write ON outlook_busy FOR ALL USING(tenant_id=app_current_tenant() AND (user_id=app_current_user() OR app_current_user()='system') AND app_has_permission('calendars:use')) WITH CHECK(tenant_id=app_current_tenant() AND (user_id=app_current_user() OR app_current_user()='system') AND app_has_permission('calendars:use'));
UPDATE role_setting SET permissions=array_append(permissions,'calendars:use') WHERE role IN ('owner','admin','recruiter','sourcer','hiring_manager') AND NOT('calendars:use'=ANY(permissions));
-- +goose Down
UPDATE role_setting SET permissions=array_remove(permissions,'calendars:use');
DROP TABLE outlook_busy,outlook_availability,calendar_event,calendar_oauth_state,calendar_connection;
