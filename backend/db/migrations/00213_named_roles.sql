-- identity (1.62.0): named roles. Owners add roles of their own ("Junior
-- recruiter"), each built on one of the five editable roles (its rank: who
-- may manage whom) with its own permissions and candidate scope. A person
-- given one gets its access instead of their base role's.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE named_role (
  id                text PRIMARY KEY DEFAULT new_id('role') CHECK (id ~ '^role_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id         text NOT NULL REFERENCES tenant (id),
  version           integer NOT NULL DEFAULT 1,
  name              text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
  base_role         text NOT NULL CHECK (base_role IN ('admin', 'recruiter', 'sourcer', 'hiring_manager', 'viewer')),
  permissions       text[] NOT NULL,
  known_permissions text[] NOT NULL,
  candidate_scope   text NOT NULL DEFAULT 'all' CHECK (candidate_scope IN ('all', 'team', 'own')),
  job_candidates    boolean NOT NULL DEFAULT true,
  updated_by        text NOT NULL,
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX named_role_name_idx ON named_role (tenant_id, lower(name));
ALTER TABLE named_role ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON named_role
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- A role in use is not removed (the service says who has it); RESTRICT keeps
-- it so even if something else tries.
ALTER TABLE app_user ADD COLUMN named_role_id text,
  ADD FOREIGN KEY (tenant_id, named_role_id) REFERENCES named_role (tenant_id, id) ON DELETE RESTRICT;
CREATE INDEX app_user_named_role_idx ON app_user (tenant_id, named_role_id) WHERE named_role_id IS NOT NULL;

-- +goose Down
ALTER TABLE app_user DROP COLUMN IF EXISTS named_role_id;
DROP TABLE IF EXISTS named_role;
