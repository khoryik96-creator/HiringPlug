-- Search by meaning (meaning module): each candidate read by a small meaning
-- model in the AI worker, so a search finds "Talent Acquisition Partner" for
-- "recruiter", and a CV that says "hired 120 engineers a year" without the
-- word. Vectors (pgvector) of 384 numbers; the database compares them.
-- meaning_candidate: what was read of each candidate (a fingerprint of their
--   titles, work and newest CV), with a vector of their whole profile.
-- meaning_role: each title they held, read two ways: as its field
--   ("Accounts Executive" is accounting) and with the work they did in it.
-- meaning_passage: passages of their newest CV (copies of CV text: removed
--   with the CV or the candidate).
-- meaning_query: a search (or a job) as read, kept so the same words are
--   not read again; meaning_query_need: the needs a job description lists.
-- meaning_sweep: where each company's read-through has got to.
-- job_claim_kinds: lets the AI worker keep one slot for searches, so a
--   recruiter's search never waits behind a long AI call.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE meaning_candidate (
  tenant_id       text NOT NULL REFERENCES tenant (id),
  candidate_id    text NOT NULL,
  fingerprint     text NOT NULL CHECK (length(fingerprint) <= 100),
  model           text NOT NULL CHECK (length(model) <= 100),
  profile         vector(384),
  read_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, candidate_id),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX meaning_candidate_profile_idx ON meaning_candidate USING hnsw (profile vector_cosine_ops);

CREATE TABLE meaning_role (
  tenant_id    text NOT NULL REFERENCES tenant (id),
  candidate_id text NOT NULL,
  ordinal      smallint NOT NULL CHECK (ordinal BETWEEN 0 AND 30), -- 0 = newest
  title        text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  family       text CHECK (length(family) <= 60),
  level        smallint CHECK (level BETWEEN 0 AND 7),
  end_year     smallint CHECK (end_year BETWEEN 1950 AND 2100), -- null: their job now
  field        vector(384) NOT NULL,
  work         vector(384) NOT NULL,
  PRIMARY KEY (tenant_id, candidate_id, ordinal),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX meaning_role_field_idx ON meaning_role USING hnsw (field vector_cosine_ops);
CREATE INDEX meaning_role_work_idx ON meaning_role USING hnsw (work vector_cosine_ops);

CREATE TABLE meaning_passage (
  tenant_id    text NOT NULL REFERENCES tenant (id),
  candidate_id text NOT NULL,
  ordinal      smallint NOT NULL CHECK (ordinal BETWEEN 0 AND 40),
  document_id  text NOT NULL REFERENCES document (id) ON DELETE CASCADE,
  passage      text NOT NULL CHECK (length(passage) BETWEEN 1 AND 1500),
  vec          vector(384) NOT NULL,
  PRIMARY KEY (tenant_id, candidate_id, ordinal),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX meaning_passage_vec_idx ON meaning_passage USING hnsw (vec vector_cosine_ops);
CREATE INDEX meaning_passage_document_idx ON meaning_passage (document_id);

CREATE TABLE meaning_query (
  id          text PRIMARY KEY DEFAULT new_id('mnq') CHECK (id ~ '^mnq_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  text_hash   text NOT NULL CHECK (length(text_hash) = 64),
  title       text CHECK (length(title) <= 300),
  body        text NOT NULL CHECK (length(body) <= 20000),
  status      text NOT NULL CHECK (status IN ('reading', 'ready', 'failed')),
  role        vector(384),  -- the role asked for, read as its field
  whole       vector(384),  -- the whole search
  context     vector(384),  -- what else it says ("for finance roles")
  level       smallint CHECK (level BETWEEN 0 AND 7),
  describes   boolean NOT NULL DEFAULT false, -- describes work rather than naming a title
  exact       boolean NOT NULL DEFAULT false, -- names a tool, qualification or code (S/4HANA): its words count most
  understood  jsonb,        -- in words: the role, its family, the level, the rest
  created_at  timestamptz NOT NULL DEFAULT now(),
  read_at     timestamptz,
  used_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, text_hash)
);

CREATE TABLE meaning_query_need (
  tenant_id text NOT NULL REFERENCES tenant (id),
  query_id  text NOT NULL REFERENCES meaning_query (id) ON DELETE CASCADE,
  ordinal   smallint NOT NULL CHECK (ordinal BETWEEN 0 AND 11),
  need      text NOT NULL CHECK (length(need) BETWEEN 1 AND 600),
  vec       vector(384) NOT NULL,
  PRIMARY KEY (query_id, ordinal)
);

CREATE TABLE meaning_sweep (
  tenant_id  text PRIMARY KEY REFERENCES tenant (id),
  chain      integer NOT NULL DEFAULT 1,
  cursor_id  text NOT NULL DEFAULT '',   -- the read-through goes by candidate id
  passes     integer NOT NULL DEFAULT 0, -- read-throughs finished
  due_at     timestamptz NOT NULL DEFAULT now(),
  ran_at     timestamptz,
  model      text CHECK (length(model) <= 100), -- the model that read the latest; a new one reads everyone again
  unavailable_until timestamptz                 -- the AI worker said it has no meaning model: ask again then
);

-- A candidate's read queued and not yet back (so a sweep does not queue it twice).
CREATE TABLE meaning_pending (
  tenant_id    text NOT NULL REFERENCES tenant (id),
  candidate_id text NOT NULL,
  fingerprint  text NOT NULL CHECK (length(fingerprint) <= 100),
  queued_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, candidate_id),
  FOREIGN KEY (tenant_id, candidate_id) REFERENCES candidate (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE meaning_candidate ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_candidate
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON meaning_candidate AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
ALTER TABLE meaning_role ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_role
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON meaning_role AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
ALTER TABLE meaning_passage ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_passage
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
CREATE POLICY candidate_scope ON meaning_passage AS RESTRICTIVE FOR SELECT USING (app_sees_candidate(candidate_id));
ALTER TABLE meaning_query ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_query
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE meaning_query_need ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_query_need
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE meaning_sweep ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_sweep
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE meaning_pending ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON meaning_pending
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- Like job_claim, but only the given kinds (the AI worker's search slot).
CREATE FUNCTION job_claim_kinds(p_queue text, p_worker text, p_lease_seconds integer, p_kinds text[])
RETURNS SETOF job
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE job
     SET status = 'running',
         attempts = attempts + 1,
         locked_by = p_worker,
         locked_until = now() + make_interval(secs => p_lease_seconds),
         updated_at = now()
   WHERE id = (
           SELECT id FROM job
            WHERE queue = p_queue AND kind = ANY (p_kinds)
              AND ((status = 'pending' AND run_after <= now())
                OR (status = 'running' AND locked_until < now()))  -- a worker died mid-job
            ORDER BY priority, run_after, id
            LIMIT 1
              FOR UPDATE SKIP LOCKED
         )
  RETURNING *
$$;
REVOKE ALL ON FUNCTION job_claim_kinds(text, text, integer, text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION job_claim_kinds(text, text, integer, text[]) TO hiringplug_app, hiringplug_ai;

-- Every company starts its read-through (the meaning.sweep job keeps itself going).
INSERT INTO meaning_sweep (tenant_id) SELECT id FROM tenant;
SELECT job_enqueue(id, 'core', 'meaning.sweep', '{"chain": 1}'::jsonb, NULL, now(), p_priority => 9::smallint) FROM tenant;

-- +goose Down
DROP FUNCTION IF EXISTS job_claim_kinds(text, text, integer, text[]);
DELETE FROM job WHERE kind LIKE 'meaning.%';
DROP TABLE IF EXISTS meaning_pending;
DROP TABLE IF EXISTS meaning_sweep;
DROP TABLE IF EXISTS meaning_query_need;
DROP TABLE IF EXISTS meaning_query;
DROP TABLE IF EXISTS meaning_passage;
DROP TABLE IF EXISTS meaning_role;
DROP TABLE IF EXISTS meaning_candidate;
