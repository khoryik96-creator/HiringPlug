-- identity module: tenants (companies using HiringPlug) and their users.

-- +goose Up
CREATE TABLE tenant (
  id         text PRIMARY KEY DEFAULT new_id('tnt') CHECK (id ~ '^tnt_[0-9a-hjkmnp-tv-z]{26}$'),
  name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app_user (
  id         text PRIMARY KEY DEFAULT new_id('usr') CHECK (id ~ '^usr_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  email      text NOT NULL CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
  name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
  role       text NOT NULL CHECK (role IN ('owner', 'admin', 'recruiter', 'sourcer', 'hiring_manager', 'viewer')),
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, email),
  UNIQUE (tenant_id, id)
);

-- Row-level security: a transaction only ever sees its own tenant.
ALTER TABLE tenant ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant
  USING (id = app_current_tenant()) WITH CHECK (id = app_current_tenant());

ALTER TABLE app_user ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON app_user
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS app_user;
DROP TABLE IF EXISTS tenant;
