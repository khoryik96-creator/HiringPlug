-- auditlog module (1.45.0): the audit log page, and who was shown a salary.
-- audit_log (platform's): indexes for the company-wide list, newest first,
--   and for one person's entries.
-- salary_view: who was shown a candidate's salary, one line per person,
--   kind, candidate and day (the agency's time zone), with how often. Never
--   an amount. Written only through app_record_salary_view (no policy lets
--   the app add, change or delete a line itself), never deleted.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE INDEX audit_log_tenant_time_idx ON audit_log (tenant_id, created_at DESC, id DESC);
CREATE INDEX audit_log_actor_time_idx ON audit_log (tenant_id, actor_id, created_at DESC, id DESC);

CREATE TABLE salary_view (
  id            text PRIMARY KEY DEFAULT new_id('sv') CHECK (id ~ '^sv_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  -- Who was shown it: a user (usr_...) or an API key (akey_...); never the system.
  user_id       text NOT NULL CHECK (length(user_id) BETWEEN 1 AND 40 AND user_id <> 'system'),
  kind          text NOT NULL CHECK (kind IN ('candidate', 'candidate_list', 'export', 'email_fields', 'email_sent', 'cv',
                  'screening', 'offer', 'placement')),
  -- Whose salary; NULL for a list, search or export (candidate_ids holds them).
  candidate_id  text CHECK (length(candidate_id) <= 40),
  candidate_ids text[] NOT NULL DEFAULT '{}',
  day           date NOT NULL,
  count         integer NOT NULL DEFAULT 1 CHECK (count >= 1),
  first_at      timestamptz NOT NULL,
  last_at       timestamptz NOT NULL,
  CHECK ((kind IN ('candidate_list', 'export')) = (candidate_id IS NULL))
);
CREATE UNIQUE INDEX salary_view_line ON salary_view (tenant_id, user_id, kind, (coalesce(candidate_id, '')), day);
CREATE INDEX salary_view_day_idx ON salary_view (tenant_id, day DESC, id DESC);
CREATE INDEX salary_view_candidate_idx ON salary_view (tenant_id, candidate_id) WHERE candidate_id IS NOT NULL;

ALTER TABLE salary_view ENABLE ROW LEVEL SECURITY;
-- Read only, by those who may see it, and only lines about candidates they
-- see: a list or export line (no candidate_id) when they see at least one of
-- its candidates (the auditlog queries count only those).
CREATE POLICY tenant_read ON salary_view FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('auditlog.salary_views:read'));
CREATE POLICY candidate_scope ON salary_view AS RESTRICTIVE FOR SELECT
  USING (CASE WHEN candidate_id IS NOT NULL THEN app_sees_candidate(candidate_id)
              ELSE app_scope_all() OR EXISTS (SELECT 1 FROM unnest(candidate_ids) AS c (id) WHERE app_sees_candidate(c.id)) END);

-- Counts one showing of a salary to the transaction's user, in their
-- company: the day's line for (user, kind, candidate) is added or counted
-- again. A list or export (no candidate) keeps the candidates it showed, so
-- the line says how many different ones that day. Nothing for the system.
-- +goose StatementBegin
CREATE FUNCTION app_record_salary_view(p_kind text, p_candidate_id text, p_candidate_ids text[], p_day date, p_at timestamptz)
RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  v_tenant text := app_current_tenant();
  v_user text := app_current_user();
BEGIN
  IF v_tenant IS NULL OR v_user IS NULL OR v_user = 'system' THEN
    RETURN;
  END IF;
  INSERT INTO salary_view AS s (tenant_id, user_id, kind, candidate_id, candidate_ids, day, first_at, last_at)
  VALUES (v_tenant, v_user, p_kind, p_candidate_id,
          ARRAY(SELECT DISTINCT unnest(coalesce(p_candidate_ids, '{}')) ORDER BY 1), p_day, p_at, p_at)
  ON CONFLICT (tenant_id, user_id, kind, (coalesce(candidate_id, '')), day) DO UPDATE
    SET count = s.count + 1,
        candidate_ids = ARRAY(SELECT DISTINCT unnest(s.candidate_ids || EXCLUDED.candidate_ids) ORDER BY 1),
        first_at = least(s.first_at, EXCLUDED.first_at),
        last_at = greatest(s.last_at, EXCLUDED.last_at);
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_record_salary_view(text, text, text[], date, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_record_salary_view(text, text, text[], date, timestamptz) TO hiringplug_app;

-- A candidate merge moves the duplicate's salary views to the kept
-- candidate: a line of theirs for the same person, kind and day takes the
-- duplicate's count (and the duplicate's line goes, counted there); list
-- and export lines name the kept candidate instead. Only for someone
-- allowed to merge candidates (the app cannot change a line otherwise).
-- +goose StatementBegin
CREATE FUNCTION app_move_salary_views(p_from text, p_to text) RETURNS void
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  v_tenant text := app_current_tenant();
BEGIN
  IF v_tenant IS NULL OR NOT app_has_permission('candidates:merge') THEN
    RAISE EXCEPTION 'not allowed to merge candidates' USING ERRCODE = '42501';
  END IF;
  UPDATE salary_view k SET count = k.count + d.count, first_at = least(k.first_at, d.first_at), last_at = greatest(k.last_at, d.last_at)
  FROM salary_view d
  WHERE k.tenant_id = v_tenant AND d.tenant_id = v_tenant AND k.candidate_id = p_to AND d.candidate_id = p_from
    AND k.user_id = d.user_id AND k.kind = d.kind AND k.day = d.day;
  DELETE FROM salary_view d
  WHERE d.tenant_id = v_tenant AND d.candidate_id = p_from AND EXISTS (
    SELECT 1 FROM salary_view k
    WHERE k.tenant_id = v_tenant AND k.candidate_id = p_to AND k.user_id = d.user_id AND k.kind = d.kind AND k.day = d.day);
  UPDATE salary_view SET candidate_id = p_to WHERE tenant_id = v_tenant AND candidate_id = p_from;
  UPDATE salary_view SET candidate_ids = ARRAY(SELECT DISTINCT unnest(array_replace(candidate_ids, p_from, p_to)) ORDER BY 1)
  WHERE tenant_id = v_tenant AND p_from = ANY (candidate_ids);
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION app_move_salary_views(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_move_salary_views(text, text) TO hiringplug_app;

-- Admin roles a company has customised get the new permissions, as they would by default.
UPDATE role_setting SET permissions = array_append(permissions, 'auditlog:read')
WHERE role = 'admin' AND NOT ('auditlog:read' = ANY (permissions));
UPDATE role_setting SET permissions = array_append(permissions, 'auditlog.salary_views:read')
WHERE role = 'admin' AND NOT ('auditlog.salary_views:read' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(array_remove(permissions, 'auditlog:read'), 'auditlog.salary_views:read');
DROP FUNCTION IF EXISTS app_move_salary_views(text, text);
DROP FUNCTION IF EXISTS app_record_salary_view(text, text, text[], date, timestamptz);
DROP TABLE IF EXISTS salary_view;
DROP INDEX IF EXISTS audit_log_actor_time_idx;
DROP INDEX IF EXISTS audit_log_tenant_time_idx;
