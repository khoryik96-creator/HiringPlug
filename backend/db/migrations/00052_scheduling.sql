-- scheduling: booking links (a small Calendly of our own).
-- schedule: each user's hours for calls, and the private address of their
--   own calendar (Google Calendar's secret iCal address or an Outlook
--   published calendar), never shown back.
-- calendar_busy: the busy times read from that address (replaced on each read).
-- booking_link: a link sent to a candidate; only a hash of its secret token
--   is kept. app_booking_tenant() turns a token's hash into its company for
--   the public booking page, like app_careers_tenant().
-- emails: an email may carry a calendar invitation (.ics).
-- identity: customised roles get scheduling:use.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE schedule (
  tenant_id         text NOT NULL REFERENCES tenant (id),
  user_id           text NOT NULL,
  time_zone         text NOT NULL CHECK (length(time_zone) BETWEEN 1 AND 64),
  hours             jsonb NOT NULL DEFAULT '[]',
  buffer_minutes    integer NOT NULL DEFAULT 10 CHECK (buffer_minutes BETWEEN 0 AND 120),
  min_notice_hours  integer NOT NULL DEFAULT 12 CHECK (min_notice_hours BETWEEN 0 AND 336),
  max_days_ahead    integer NOT NULL DEFAULT 14 CHECK (max_days_ahead BETWEEN 1 AND 60),
  calendar_url      text CHECK (length(calendar_url) <= 2000),
  calendar_status   text CHECK (calendar_status IN ('pending', 'ok', 'failed')),
  calendar_error    text CHECK (length(calendar_error) <= 500),
  calendar_read_at  timestamptz,
  calendar_asked_at timestamptz, -- when a new read was last queued
  updated_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);
CREATE TABLE calendar_busy (
  tenant_id  text NOT NULL REFERENCES tenant (id),
  user_id    text NOT NULL,
  starts_at  timestamptz NOT NULL,
  ends_at    timestamptz NOT NULL CHECK (ends_at > starts_at)
);
CREATE INDEX calendar_busy_user_idx ON calendar_busy (tenant_id, user_id, starts_at);

CREATE TABLE booking_link (
  id                text PRIMARY KEY DEFAULT new_id('bkl') CHECK (id ~ '^bkl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id         text NOT NULL REFERENCES tenant (id),
  token_hash        text NOT NULL UNIQUE,
  host_user_id      text NOT NULL,
  candidate_id      text NOT NULL,
  application_id    text,
  title             text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
  duration_minutes  integer NOT NULL CHECK (duration_minutes IN (15, 30, 45, 60, 90)),
  location          text CHECK (length(location) <= 500),
  status            text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'booked', 'cancelled')),
  starts_at         timestamptz,
  sequence          integer NOT NULL DEFAULT 0,
  expires_at        timestamptz NOT NULL,
  created_by        text NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  CHECK ((status = 'booked') = (starts_at IS NOT NULL) OR status = 'cancelled')
);
CREATE INDEX booking_link_candidate_idx ON booking_link (tenant_id, candidate_id, created_at DESC);
CREATE INDEX booking_link_host_idx ON booking_link (tenant_id, host_user_id, starts_at) WHERE status = 'booked';

ALTER TABLE schedule ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON schedule
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE calendar_busy ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON calendar_busy
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE booking_link ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON booking_link
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- The public booking page knows only the link's token. This turns its hash
-- into the company, and returns nothing else; everything after runs in that
-- company's tenant scope, where row-level security applies as usual.
CREATE FUNCTION app_booking_tenant(p_token_hash text) RETURNS TABLE (tenant_id text, tenant_name text)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT t.id, t.name FROM booking_link b JOIN tenant t ON t.id = b.tenant_id WHERE b.token_hash = p_token_hash
$$;
REVOKE ALL ON FUNCTION app_booking_tenant(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_booking_tenant(text) TO hiringplug_app;

ALTER TABLE email ADD COLUMN calendar text CHECK (length(calendar) <= 20000);

-- Roles a company has customised get scheduling:use as they would by default.
UPDATE role_setting SET permissions = array_append(permissions, 'scheduling:use')
WHERE role IN ('owner', 'admin', 'recruiter', 'sourcer', 'hiring_manager') AND NOT ('scheduling:use' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'scheduling:use');
ALTER TABLE email DROP COLUMN IF EXISTS calendar;
DROP FUNCTION IF EXISTS app_booking_tenant(text);
DROP TABLE IF EXISTS booking_link;
DROP TABLE IF EXISTS calendar_busy;
DROP TABLE IF EXISTS schedule;
