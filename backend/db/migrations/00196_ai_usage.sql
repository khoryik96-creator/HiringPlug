-- aiusage module: one ledger of every AI use (what each feature spent, when
-- and for whom), each company's monthly AI limit and prices, and the months
-- a limit warning was reached.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up

-- The ledger. Append-only: no purge, Bin, merge or "forget" deletes a row, so
-- spent tokens stay spent when the record they were for is gone (there are
-- no foreign keys to the records for that reason). A row holds no personal
-- data beyond user_id (who asked; NULL when the system ran it on its own).
--
-- (tenant_id, feature, source_id, run_key) is the idempotency key: source_id
-- is the record the use belongs to (an answer, a fit score, a CV, a JD
-- file...) and run_key tells one AI run of that record from another (empty
-- for records the AI handles only once). The same result delivered twice
-- has the same key and is not counted again; a new run of the same record
-- has a new run_key and counts.
CREATE TABLE ai_use (
  id            text PRIMARY KEY DEFAULT new_id('aiu') CHECK (id ~ '^aiu_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id     text NOT NULL REFERENCES tenant (id),
  feature       text NOT NULL CHECK (feature IN ('assistant', 'assistant_email', 'fit_score', 'talent_search', 'cv_reading',
                  'cv_studio', 'jd_blind', 'jd_reading', 'industry_pick', 'interview_prep', 'provider_check')),
  user_id       text CHECK (length(user_id) <= 40),
  source_id     text NOT NULL CHECK (length(source_id) BETWEEN 1 AND 80),
  run_key       text NOT NULL DEFAULT '' CHECK (length(run_key) <= 80),
  input_tokens  bigint NOT NULL CHECK (input_tokens >= 0),
  output_tokens bigint NOT NULL CHECK (output_tokens >= 0),
  used_at       timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ai_use_run_key ON ai_use (tenant_id, feature, source_id, run_key);
CREATE INDEX ai_use_month_idx ON ai_use (tenant_id, used_at);

ALTER TABLE ai_use ENABLE ROW LEVEL SECURITY;
-- Read and add only: no policy lets anyone change or delete a use.
CREATE POLICY tenant_read ON ai_use FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY tenant_insert ON ai_use FOR INSERT WITH CHECK (tenant_id = app_current_tenant());

-- Each company's limit and prices. No row: the server's default limit in
-- tokens (HP_AI_MONTHLY_TOKENS) and no prices.
CREATE TABLE ai_usage_setting (
  tenant_id                text PRIMARY KEY REFERENCES tenant (id),
  limit_kind               text NOT NULL DEFAULT 'tokens' CHECK (limit_kind IN ('tokens', 'money')),
  limit_tokens             bigint CHECK (limit_tokens >= 0),
  limit_money              numeric(14, 2) CHECK (limit_money >= 0),
  currency                 text NOT NULL DEFAULT 'USD' CHECK (currency ~ '^[A-Z]{3}$'),
  price_input_per_million  numeric(14, 4) CHECK (price_input_per_million >= 0),
  price_output_per_million numeric(14, 4) CHECK (price_output_per_million >= 0),
  warn_percent             smallint NOT NULL DEFAULT 80 CHECK (warn_percent BETWEEN 1 AND 99),
  version                  integer NOT NULL DEFAULT 1,
  updated_by               text,
  updated_at               timestamptz NOT NULL DEFAULT now(),
  CHECK (limit_kind = 'tokens' OR (limit_money IS NOT NULL AND price_input_per_million IS NOT NULL AND price_output_per_million IS NOT NULL))
);

ALTER TABLE ai_usage_setting ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON ai_usage_setting FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY manage_insert ON ai_usage_setting FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('aiusage:manage'));
CREATE POLICY manage_update ON ai_usage_setting FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('aiusage:manage'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('aiusage:manage'));
CREATE POLICY manage_delete ON ai_usage_setting FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('aiusage:manage'));

-- When a month's use first reached the warning level or the whole limit
-- (level is the percent: the company's warning percent, or 100). One row per
-- month and level, so each is announced once.
CREATE TABLE ai_usage_alert (
  tenant_id  text NOT NULL REFERENCES tenant (id),
  month      date NOT NULL CHECK (extract(day FROM month) = 1),
  level      smallint NOT NULL CHECK (level BETWEEN 1 AND 100),
  reached_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, month, level)
);

ALTER TABLE ai_usage_alert ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_usage_alert
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- Everything already spent, from each feature's own records (all history),
-- keeping who asked and the input/output split where the record has them.
-- Counted by when the record was made, as the monthly sums were until now.
-- "system" (automatic work) becomes NULL. Records that spent nothing are left out.
INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'assistant', nullif(user_id, 'system'), id, input_tokens, output_tokens, created_at
FROM assistant_answer WHERE input_tokens + output_tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'assistant_email', nullif(user_id, 'system'), id, input_tokens, output_tokens, created_at
FROM assistant_email WHERE input_tokens + output_tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'fit_score', nullif(created_by, 'system'), id, input_tokens, output_tokens, created_at
FROM job_match WHERE input_tokens + output_tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'talent_search', nullif(created_by, 'system'), id, tokens, 0, created_at
FROM talent_search WHERE tokens > 0;

-- document_ai_use and studio_cv_ai_use have no id: a row per reading of the
-- document (or run of the formatted CV), numbered to keep each one.
INSERT INTO ai_use (tenant_id, feature, user_id, source_id, run_key, input_tokens, output_tokens, used_at)
SELECT u.tenant_id, 'cv_reading', nullif(d.created_by, 'system'), u.document_id,
       'before-1.44:' || row_number() OVER (PARTITION BY u.tenant_id, u.document_id ORDER BY u.created_at),
       u.tokens, 0, u.created_at
FROM document_ai_use u
LEFT JOIN document d ON d.tenant_id = u.tenant_id AND d.id = u.document_id
WHERE u.tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, run_key, input_tokens, output_tokens, used_at)
SELECT u.tenant_id, 'cv_studio', nullif(s.created_by, 'system'), u.studio_cv_id,
       'before-1.44:' || row_number() OVER (PARTITION BY u.tenant_id, u.studio_cv_id ORDER BY u.created_at),
       u.tokens, 0, u.created_at
FROM studio_cv_ai_use u
LEFT JOIN studio_cv s ON s.tenant_id = u.tenant_id AND s.id = u.studio_cv_id
WHERE u.tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'jd_blind', nullif(created_by, 'system'), id, tokens, 0, created_at
FROM job_ad_blind_rewrite WHERE tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'jd_reading', nullif(created_by, 'system'), id, ai_tokens, 0, created_at
FROM job_description_file WHERE ai_tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'industry_pick', NULL, id, input_tokens, output_tokens, created_at
FROM company_industry_pick WHERE input_tokens + output_tokens > 0;

INSERT INTO ai_use (tenant_id, feature, user_id, source_id, input_tokens, output_tokens, used_at)
SELECT tenant_id, 'interview_prep', nullif(created_by, 'system'), id, greatest(tokens, 0), 0, created_at
FROM interview_prep WHERE tokens > 0;

-- Admin roles a company has customised get the new permissions, as they would by default.
UPDATE role_setting SET permissions = array_append(permissions, 'aiusage:read')
WHERE role = 'admin' AND NOT ('aiusage:read' = ANY (permissions));
UPDATE role_setting SET permissions = array_append(permissions, 'aiusage:manage')
WHERE role = 'admin' AND NOT ('aiusage:manage' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(array_remove(permissions, 'aiusage:read'), 'aiusage:manage');
DROP TABLE IF EXISTS ai_usage_alert;
DROP TABLE IF EXISTS ai_usage_setting;
DROP TABLE IF EXISTS ai_use;
