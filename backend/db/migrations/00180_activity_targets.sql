-- reports module: activity targets per user (or one for everyone) per week
-- or month, and the revenue forecast's weight for each stage. Both are
-- company settings that owners and admins change (reports.settings:manage);
-- the database refuses changes from anyone else.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE activity_target (
  id          text PRIMARY KEY DEFAULT new_id('atg') CHECK (id ~ '^atg_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  user_id     text CHECK (length(user_id) <= 40), -- null: everyone without their own
  period      text NOT NULL CHECK (period IN ('week', 'month')),
  -- Null: no target for that activity.
  calls       integer CHECK (calls BETWEEN 0 AND 100000),
  cvs_sent    integer CHECK (cvs_sent BETWEEN 0 AND 100000),
  interviews  integer CHECK (interviews BETWEEN 0 AND 100000),
  placements  integer CHECK (placements BETWEEN 0 AND 100000),
  updated_by  text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX activity_target_key ON activity_target (tenant_id, coalesce(user_id, ''), period);

CREATE TABLE forecast_stage_weight (
  tenant_id    text NOT NULL REFERENCES tenant (id),
  stage        text NOT NULL CHECK (stage IN ('submitted', 'interview', 'offer', 'placed')),
  percent      integer NOT NULL CHECK (percent BETWEEN 0 AND 100),
  months_ahead integer NOT NULL CHECK (months_ahead BETWEEN 0 AND 12),
  updated_by   text NOT NULL,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, stage)
);

-- Every table holding tenant data has row-level security (a test checks this).
-- Everyone in the company reads them (a recruiter sees their own target);
-- only reports.settings:manage changes them.
ALTER TABLE activity_target ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON activity_target FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY manage_insert ON activity_target FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'));
CREATE POLICY manage_update ON activity_target FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'));
CREATE POLICY manage_delete ON activity_target FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'));

ALTER TABLE forecast_stage_weight ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON forecast_stage_weight FOR SELECT USING (tenant_id = app_current_tenant());
CREATE POLICY manage_insert ON forecast_stage_weight FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'));
CREATE POLICY manage_update ON forecast_stage_weight FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'));
CREATE POLICY manage_delete ON forecast_stage_weight FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('reports.settings:manage'));

-- Admin roles a company has customised get the new permissions, as they would by default.
UPDATE role_setting SET permissions = array_append(permissions, 'reports.team:read')
WHERE role = 'admin' AND NOT ('reports.team:read' = ANY (permissions));
UPDATE role_setting SET permissions = array_append(permissions, 'reports.settings:manage')
WHERE role = 'admin' AND NOT ('reports.settings:manage' = ANY (permissions));
UPDATE role_setting SET permissions = array_append(permissions, 'reports.forecast:read')
WHERE role = 'admin' AND NOT ('reports.forecast:read' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(array_remove(array_remove(permissions,
  'reports.team:read'), 'reports.settings:manage'), 'reports.forecast:read');
DROP TABLE IF EXISTS forecast_stage_weight;
DROP TABLE IF EXISTS activity_target;
