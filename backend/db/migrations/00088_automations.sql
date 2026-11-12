-- automations: When · If · Then rules (automation), what they did or will do
-- (automation_run), recruiters' tasks (recruiter_task) and the company's
-- default (automation_settings). Events come from the audit log, as for
-- webhooks: an entry an enabled automation may listen for queues an
-- automation.event job in the same transaction as the change.
-- identity: admins who chose their own permissions get the new ones.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE automation_settings (
  tenant_id         text PRIMARY KEY REFERENCES tenant (id),
  default_send_mode text NOT NULL DEFAULT 'approve' CHECK (default_send_mode IN ('send', 'approve')),
  seeded            boolean NOT NULL DEFAULT false, -- the ready-made automations were added
  updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE automation (
  id          text PRIMARY KEY DEFAULT new_id('aut') CHECK (id ~ '^aut_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id   text NOT NULL REFERENCES tenant (id),
  version     integer NOT NULL DEFAULT 1,
  name        text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  enabled     boolean NOT NULL DEFAULT false,
  trigger     text NOT NULL CHECK (trigger IN ('application_received', 'stage_changed', 'interview_scheduled',
                                               'placement_starts', 'guarantee_ends', 'invoice_due')),
  stage       text CHECK (stage IN ('applied', 'screening', 'submitted', 'interview', 'offer', 'placed', 'rejected', 'withdrawn')),
  offset_days integer NOT NULL DEFAULT 0 CHECK (offset_days BETWEEN -60 AND 365),
  company_id  text,
  actions     jsonb NOT NULL CHECK (jsonb_typeof(actions) = 'array' AND jsonb_array_length(actions) BETWEEN 1 AND 3),
  send_mode   text NOT NULL DEFAULT 'approve' CHECK (send_mode IN ('send', 'approve')),
  preset      text CHECK (preset ~ '^[a-z0-9_]{1,40}$'),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  CONSTRAINT automation_stage CHECK ((trigger = 'stage_changed') = (stage IS NOT NULL))
);
CREATE INDEX automation_enabled_idx ON automation (tenant_id) WHERE enabled;

-- One run per automation and record (an application, interview or
-- placement): a moved date moves the run; it never repeats.
CREATE TABLE automation_run (
  id             text PRIMARY KEY DEFAULT new_id('arn') CHECK (id ~ '^arn_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL,
  automation_id  text NOT NULL,
  subject_type   text NOT NULL CHECK (subject_type IN ('application', 'interview', 'placement')),
  subject_id     text NOT NULL,
  application_id text NOT NULL,
  due_at         timestamptz NOT NULL,
  status         text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'to_approve', 'done', 'skipped', 'failed')),
  detail         text CHECK (length(detail) <= 2000),
  created_at     timestamptz NOT NULL DEFAULT now(),
  finished_at    timestamptz,
  finished_by    text,
  UNIQUE (automation_id, subject_id),
  FOREIGN KEY (tenant_id, automation_id) REFERENCES automation (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX automation_run_tenant_idx ON automation_run (tenant_id, id DESC);
CREATE INDEX automation_run_status_idx ON automation_run (tenant_id, status, due_at);

CREATE TABLE recruiter_task (
  id             text PRIMARY KEY DEFAULT new_id('tsk') CHECK (id ~ '^tsk_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  user_id        text NOT NULL,
  title          text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
  due_on         date NOT NULL,
  application_id text,
  run_id         text REFERENCES automation_run (id) ON DELETE SET NULL,
  automation_name text,
  done_at        timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recruiter_task_user_idx ON recruiter_task (tenant_id, user_id, done_at, due_on);

-- Every table holding tenant data has row-level security (a test checks this).
ALTER TABLE automation_settings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON automation_settings
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE automation ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON automation
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE automation_run ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON automation_run
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());
ALTER TABLE recruiter_task ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recruiter_task
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- Changes an automation may start from, queued when the company has an
-- automation switched on (the worker works out which ones apply).
-- +goose StatementBegin
CREATE FUNCTION automation_fan_out() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.action IN ('application.created', 'application.moved', 'interview.scheduled', 'interview.updated',
                    'placement.recorded', 'placement.updated', 'placement.invoicing_set')
     AND EXISTS (SELECT 1 FROM automation WHERE tenant_id = NEW.tenant_id AND enabled) THEN
    PERFORM job_enqueue(NEW.tenant_id, 'core', 'automation.event', jsonb_build_object(
      'action', NEW.action, 'entityType', NEW.entity_type, 'entityId', NEW.entity_id, 'changes', NEW.changes), NEW.request_id);
  END IF;
  RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER audit_log_automations AFTER INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION automation_fan_out();

UPDATE role_setting SET permissions = permissions || ARRAY['automations:read', 'automations:manage', 'automations:approve', 'automations.tasks:read', 'automations.tasks:update']
WHERE role = 'admin' AND NOT ('automations:read' = ANY (permissions));

-- +goose Down
UPDATE role_setting SET permissions = array_remove(array_remove(array_remove(array_remove(array_remove(permissions,
  'automations:read'), 'automations:manage'), 'automations:approve'), 'automations.tasks:read'), 'automations.tasks:update');
DROP TRIGGER IF EXISTS audit_log_automations ON audit_log;
DROP FUNCTION IF EXISTS automation_fan_out();
DROP TABLE IF EXISTS recruiter_task;
DROP TABLE IF EXISTS automation_run;
DROP TABLE IF EXISTS automation;
DROP TABLE IF EXISTS automation_settings;
