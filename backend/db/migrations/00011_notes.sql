-- notes module: notes on candidates, jobs, companies and contacts. Sensitive
-- notes (e.g. salary talks) are invisible without notes.sensitive:read.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE note (
  id           text PRIMARY KEY DEFAULT new_id('note') CHECK (id ~ '^note_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id    text NOT NULL REFERENCES tenant (id),
  -- The subject is checked by the service through its module (a note can be
  -- about different kinds of record, so there is no single foreign key).
  subject_type text NOT NULL CHECK (subject_type IN ('candidate', 'job', 'company', 'contact')),
  subject_id   text NOT NULL CHECK (length(subject_id) <= 40),
  kind         text NOT NULL DEFAULT 'general' CHECK (kind IN ('general', 'screening', 'interview', 'call')),
  body         text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 10000),
  sensitive    boolean NOT NULL DEFAULT false,
  author_id    text NOT NULL,
  author_name  text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX note_subject_idx ON note (tenant_id, subject_type, subject_id, id DESC);

-- Every table holding tenant data has row-level security (a test checks this).
-- Sensitive rows also need the permission, whatever the query.
ALTER TABLE note ENABLE ROW LEVEL SECURITY;
CREATE POLICY note_select ON note FOR SELECT
  USING (tenant_id = app_current_tenant() AND (NOT sensitive OR app_has_permission('notes.sensitive:read')));
CREATE POLICY note_insert ON note FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND (NOT sensitive OR app_has_permission('notes.sensitive:read')));
CREATE POLICY note_delete ON note FOR DELETE
  USING (tenant_id = app_current_tenant() AND (NOT sensitive OR app_has_permission('notes.sensitive:read')));

-- How many sensitive notes a subject has, for users who may not read them
-- ("2 sensitive notes hidden"). SECURITY DEFINER: it counts past the policy
-- but returns only a number, for the current tenant.
CREATE FUNCTION app_hidden_sensitive_notes(p_subject_type text, p_subject_id text) RETURNS integer
  LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
    SELECT count(*)::integer FROM note
    WHERE tenant_id = app_current_tenant() AND subject_type = p_subject_type AND subject_id = p_subject_id
      AND sensitive AND NOT app_has_permission('notes.sensitive:read')
$$;
REVOKE ALL ON FUNCTION app_hidden_sensitive_notes(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app_hidden_sensitive_notes(text, text) TO hiringplug_app;

-- +goose Down
DROP FUNCTION IF EXISTS app_hidden_sensitive_notes(text, text);
DROP TABLE IF EXISTS note;
