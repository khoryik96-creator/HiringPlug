-- Who sees which candidates. Each company chooses, per role, its permissions
-- and which candidates it sees: all, its team's or its own (plus, if chosen,
-- those on jobs it or its team owns). Candidates and jobs get an owner, and
-- users can be put in teams.
--
-- The database enforces it: the API sets app.scope_users (the users whose
-- candidates the user sees; empty = all) and app.scope_jobs ('on' = also
-- candidates on those users' jobs) in every transaction. A candidate out of
-- scope is invisible, and so is everything about them in every module
-- (restrictive SELECT policies, which also bind UPDATE and DELETE).
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
ALTER TABLE candidate ADD COLUMN owner_id text CHECK (length(owner_id) <= 40);
UPDATE candidate c SET owner_id = c.created_by
WHERE EXISTS (SELECT 1 FROM app_user u WHERE u.tenant_id = c.tenant_id AND u.id = c.created_by);
CREATE INDEX candidate_owner_idx ON candidate (tenant_id, owner_id);

ALTER TABLE job_order ADD COLUMN owner_id text CHECK (length(owner_id) <= 40);
UPDATE job_order j SET owner_id = j.created_by
WHERE EXISTS (SELECT 1 FROM app_user u WHERE u.tenant_id = j.tenant_id AND u.id = j.created_by);
CREATE INDEX job_order_owner_idx ON job_order (tenant_id, owner_id);

CREATE TABLE team_group (
  id         text PRIMARY KEY DEFAULT new_id('team') CHECK (id ~ '^team_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id  text NOT NULL REFERENCES tenant (id),
  version    integer NOT NULL DEFAULT 1,
  name       text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX team_group_name_idx ON team_group (tenant_id, lower(name));
ALTER TABLE app_user ADD COLUMN team_id text,
  ADD FOREIGN KEY (tenant_id, team_id) REFERENCES team_group (tenant_id, id) ON DELETE SET NULL (team_id);
CREATE INDEX app_user_team_idx ON app_user (tenant_id, team_id);

-- A company's own settings for a role; no row = the contract's defaults
-- (and all candidates). The owner role is never changed.
CREATE TABLE role_setting (
  tenant_id       text NOT NULL REFERENCES tenant (id),
  role            text NOT NULL CHECK (role IN ('admin', 'recruiter', 'sourcer', 'hiring_manager', 'viewer')),
  permissions     text[] NOT NULL,
  candidate_scope text NOT NULL DEFAULT 'all' CHECK (candidate_scope IN ('all', 'team', 'own')),
  job_candidates  boolean NOT NULL DEFAULT true,
  version         integer NOT NULL DEFAULT 1,
  updated_by      text NOT NULL,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, role)
);

ALTER TABLE team_group ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON team_group
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE role_setting ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON role_setting
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE FUNCTION app_scope_all() RETURNS boolean
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT coalesce(current_setting('app.scope_users', true), '') = ''
$$;

CREATE FUNCTION app_scope_users() RETURNS text[]
LANGUAGE sql STABLE PARALLEL SAFE AS $$
  SELECT string_to_array(coalesce(current_setting('app.scope_users', true), ''), ',')
$$;

-- Whether a candidate is on a job owned by someone in scope. It reads past
-- row-level security (applications refer back to candidates, so a policy
-- could not ask through them) and only ever answers yes or no.
CREATE FUNCTION app_on_scope_jobs(p_candidate_id text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  SELECT coalesce(current_setting('app.scope_jobs', true), '') = 'on' AND EXISTS (
    SELECT 1 FROM application a
    JOIN job_order j ON j.tenant_id = a.tenant_id AND j.id = a.job_order_id
    WHERE a.tenant_id = app_current_tenant() AND a.candidate_id = p_candidate_id AND j.owner_id = ANY (app_scope_users()))
$$;
REVOKE ALL ON FUNCTION app_on_scope_jobs(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_on_scope_jobs(text) TO hiringplug_app;

CREATE FUNCTION app_sees_candidate_row(p_owner_id text, p_id text) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT app_scope_all() OR coalesce(p_owner_id = ANY (app_scope_users()), false) OR app_on_scope_jobs(p_id)
$$;

-- For rows about a candidate (or an application): visible when it is.
CREATE FUNCTION app_sees_candidate(p_id text) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT app_scope_all() OR EXISTS (SELECT 1 FROM candidate WHERE id = p_id)
$$;

CREATE FUNCTION app_sees_application(p_id text) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT app_scope_all() OR EXISTS (SELECT 1 FROM application WHERE id = p_id)
$$;

-- Handing a candidate over: the new row may be out of the caller's scope,
-- which row-level security would refuse, so this changes only the owner, of
-- a candidate the caller sees (the same rule as the policy below), with
-- candidates:update.
CREATE FUNCTION app_set_candidate_owner(p_id text, p_owner_id text) RETURNS boolean
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  WITH changed AS (
    UPDATE candidate SET owner_id = p_owner_id, version = version + 1, updated_at = now()
    WHERE tenant_id = app_current_tenant() AND id = p_id
      AND app_has_permission('candidates:update') AND app_sees_candidate_row(owner_id, id)
    RETURNING 1)
  SELECT EXISTS (SELECT 1 FROM changed)
$$;
REVOKE ALL ON FUNCTION app_set_candidate_owner(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_set_candidate_owner(text, text) TO hiringplug_app;

CREATE POLICY candidate_scope ON candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate_row(owner_id, id));
CREATE POLICY candidate_scope ON candidate_compensation AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON candidate_work AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON candidate_education AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON candidate_consent AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON privacy_request AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON document AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON application AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
CREATE POLICY candidate_scope ON email AS RESTRICTIVE FOR SELECT
  USING ((candidate_id IS NULL OR app_sees_candidate(candidate_id)) AND (application_id IS NULL OR app_sees_application(application_id)));
CREATE POLICY candidate_scope ON note AS RESTRICTIVE FOR SELECT
  USING (subject_type <> 'candidate' OR app_sees_candidate(subject_id));
CREATE POLICY candidate_scope ON application_stage_change AS RESTRICTIVE FOR SELECT USING (app_sees_application(application_id));
CREATE POLICY candidate_scope ON interview AS RESTRICTIVE FOR SELECT USING (app_sees_application(application_id));
CREATE POLICY candidate_scope ON interview_feedback AS RESTRICTIVE FOR SELECT
  USING (app_scope_all() OR EXISTS (SELECT 1 FROM interview WHERE id = interview_id));
CREATE POLICY candidate_scope ON offer AS RESTRICTIVE FOR SELECT USING (app_sees_application(application_id));
CREATE POLICY candidate_scope ON offer_salary AS RESTRICTIVE FOR SELECT
  USING (app_scope_all() OR EXISTS (SELECT 1 FROM offer WHERE id = offer_id));
CREATE POLICY candidate_scope ON placement AS RESTRICTIVE FOR SELECT USING (app_sees_application(application_id));
CREATE POLICY candidate_scope ON placement_money AS RESTRICTIVE FOR SELECT
  USING (app_scope_all() OR EXISTS (SELECT 1 FROM placement WHERE id = placement_id));
CREATE POLICY candidate_scope ON job_match AS RESTRICTIVE FOR SELECT USING (app_sees_application(application_id));
CREATE POLICY candidate_scope ON assistant_answer AS RESTRICTIVE FOR SELECT
  USING (CASE subject_type WHEN 'candidate' THEN app_sees_candidate(subject_id)
                           WHEN 'application' THEN app_sees_application(subject_id) ELSE true END);
CREATE POLICY candidate_scope ON audit_log AS RESTRICTIVE FOR SELECT
  USING (CASE entity_type WHEN 'candidate' THEN app_sees_candidate(entity_id)
                          WHEN 'application' THEN app_sees_application(entity_id) ELSE true END);

-- +goose Down
DROP POLICY IF EXISTS candidate_scope ON audit_log;
DROP POLICY IF EXISTS candidate_scope ON assistant_answer;
DROP POLICY IF EXISTS candidate_scope ON job_match;
DROP POLICY IF EXISTS candidate_scope ON placement_money;
DROP POLICY IF EXISTS candidate_scope ON placement;
DROP POLICY IF EXISTS candidate_scope ON offer_salary;
DROP POLICY IF EXISTS candidate_scope ON offer;
DROP POLICY IF EXISTS candidate_scope ON interview_feedback;
DROP POLICY IF EXISTS candidate_scope ON interview;
DROP POLICY IF EXISTS candidate_scope ON application_stage_change;
DROP POLICY IF EXISTS candidate_scope ON note;
DROP POLICY IF EXISTS candidate_scope ON email;
DROP POLICY IF EXISTS candidate_scope ON application;
DROP POLICY IF EXISTS candidate_scope ON document;
DROP POLICY IF EXISTS candidate_scope ON privacy_request;
DROP POLICY IF EXISTS candidate_scope ON candidate_consent;
DROP POLICY IF EXISTS candidate_scope ON candidate_education;
DROP POLICY IF EXISTS candidate_scope ON candidate_work;
DROP POLICY IF EXISTS candidate_scope ON candidate_compensation;
DROP POLICY IF EXISTS candidate_scope ON candidate;
DROP FUNCTION IF EXISTS app_set_candidate_owner(text, text);
DROP FUNCTION IF EXISTS app_sees_application(text);
DROP FUNCTION IF EXISTS app_sees_candidate(text);
DROP FUNCTION IF EXISTS app_sees_candidate_row(text, text);
DROP FUNCTION IF EXISTS app_on_scope_jobs(text);
DROP FUNCTION IF EXISTS app_scope_users();
DROP FUNCTION IF EXISTS app_scope_all();
DROP TABLE IF EXISTS role_setting;
ALTER TABLE app_user DROP COLUMN IF EXISTS team_id;
DROP TABLE IF EXISTS team_group;
ALTER TABLE job_order DROP COLUMN IF EXISTS owner_id;
ALTER TABLE candidate DROP COLUMN IF EXISTS owner_id;
