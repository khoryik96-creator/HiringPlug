-- Client shortlist links (shortlists module): a recruiter sends a client a
-- link to some of a job's candidates, each with the CV the recruiter chose;
-- the client (no account) approves or rejects each one with a comment.
-- client_shortlist: the link; only a hash of its secret token is kept.
--   app_client_shortlist_tenant() turns a token's hash into its company for
--   the public page, like app_booking_tenant().
-- client_shortlist_candidate: who is on it, the CV shown, the client's answer.
-- formatted_cv (cvformats module): how each formatted CV was made (blind,
--   with salary), so only CVs without salary are shared. Filled from the
--   audit log for the CVs made before.
-- identity: customised owner, admin and recruiter roles get shortlists:share.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE formatted_cv (
  tenant_id      text NOT NULL REFERENCES tenant (id),
  document_id    text NOT NULL,
  candidate_id   text NOT NULL,
  blind          boolean NOT NULL,
  include_salary boolean NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, document_id)
);
CREATE INDEX formatted_cv_candidate_idx ON formatted_cv (tenant_id, candidate_id);
ALTER TABLE formatted_cv ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON formatted_cv
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON formatted_cv AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));

-- Every CV made so far: the audit log has what was chosen for each (never the values).
INSERT INTO formatted_cv (tenant_id, document_id, candidate_id, blind, include_salary, created_at)
SELECT DISTINCT ON (l.tenant_id, l.changes->>'documentId')
       l.tenant_id, l.changes->>'documentId', l.entity_id,
       coalesce((l.changes->>'blind')::boolean, false), coalesce((l.changes->>'includeSalary')::boolean, false), l.created_at
FROM audit_log l
WHERE l.action = 'cv.formatted' AND l.entity_type = 'candidate' AND l.changes ? 'documentId'
ORDER BY l.tenant_id, l.changes->>'documentId', l.created_at DESC
ON CONFLICT DO NOTHING;

CREATE TABLE client_shortlist (
  id             text PRIMARY KEY DEFAULT new_id('csl') CHECK (id ~ '^csl_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  token_hash     text NOT NULL UNIQUE,
  job_id         text NOT NULL,
  message        text CHECK (length(message) <= 2000),
  expires_at     timestamptz NOT NULL,
  revoked_at     timestamptz,
  revoked_by     text,
  opened_at      timestamptz, -- the client's first visit
  last_opened_at timestamptz,
  emailed_to     text CHECK (length(emailed_to) <= 254),
  created_by     text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX client_shortlist_job_idx ON client_shortlist (tenant_id, job_id, created_at DESC);

CREATE TABLE client_shortlist_candidate (
  id             text PRIMARY KEY DEFAULT new_id('cslc') CHECK (id ~ '^cslc_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  shortlist_id   text NOT NULL REFERENCES client_shortlist (id) ON DELETE CASCADE,
  position       smallint NOT NULL CHECK (position BETWEEN 1 AND 20),
  application_id text NOT NULL,
  candidate_id   text NOT NULL,
  document_id    text NOT NULL,
  blind          boolean NOT NULL,
  label          text NOT NULL CHECK (length(label) BETWEEN 1 AND 200), -- the name the client sees
  note           text CHECK (length(note) <= 1000),
  decision       text CHECK (decision IN ('approved', 'rejected')),
  comment        text CHECK (length(comment) <= 2000),
  answered_at    timestamptz,
  CHECK ((decision IS NULL) = (answered_at IS NULL)),
  UNIQUE (shortlist_id, application_id)
);
CREATE INDEX client_shortlist_candidate_list_idx ON client_shortlist_candidate (tenant_id, shortlist_id, position);
CREATE INDEX client_shortlist_candidate_candidate_idx ON client_shortlist_candidate (tenant_id, candidate_id);

ALTER TABLE client_shortlist ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON client_shortlist
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE client_shortlist_candidate ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON client_shortlist_candidate
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON client_shortlist_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));

-- The public page knows only the link's token. This turns its hash into the
-- company, and returns nothing else; everything after runs in that
-- company's tenant scope, where row-level security applies as usual.
CREATE FUNCTION app_client_shortlist_tenant(p_token_hash text) RETURNS TABLE (tenant_id text, tenant_name text)
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT t.id, t.name FROM client_shortlist s JOIN tenant t ON t.id = s.tenant_id WHERE s.token_hash = p_token_hash
$$;
REVOKE ALL ON FUNCTION app_client_shortlist_tenant(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_client_shortlist_tenant(text) TO hiringplug_app;

-- Roles a company has customised get shortlists:share as they would by default.
UPDATE role_setting SET permissions = array_append(permissions, 'shortlists:share')
WHERE role IN ('owner', 'admin', 'recruiter') AND NOT ('shortlists:share' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(permissions, 'shortlists:share');
DROP FUNCTION IF EXISTS app_client_shortlist_tenant(text);
DROP TABLE IF EXISTS client_shortlist_candidate;
DROP TABLE IF EXISTS client_shortlist;
DROP TABLE IF EXISTS formatted_cv;
