-- placements: a job's retainer (a retained search). The client pays in
-- stages while the search runs, each due on a date or a number of days after
-- the retainer was signed or the placement starts, each invoiced and paid;
-- what the stages add up to is taken off the fee of the first placement on
-- the job (that placement invoices the balance).
-- Money: rows are invisible without placements.fees:read and writable only
-- with placements.fees:update, like placement_money.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE job_retainer (
  job_order_id text PRIMARY KEY,
  tenant_id    text NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  currency     text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  signed_on    date,
  updated_by   text NOT NULL,
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, job_order_id),
  FOREIGN KEY (tenant_id, job_order_id) REFERENCES job_order (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE job_retainer_stage (
  id             text PRIMARY KEY DEFAULT new_id('rts') CHECK (id ~ '^rts_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL,
  job_order_id   text NOT NULL,
  position       integer NOT NULL CHECK (position BETWEEN 0 AND 11),
  label          text NOT NULL CHECK (length(label) BETWEEN 1 AND 100),
  amount         numeric(14, 2) NOT NULL CHECK (amount > 0),
  due_kind       text NOT NULL CHECK (due_kind IN ('on_date', 'after_signing', 'after_start')),
  due_on         date,
  due_days       integer CHECK (due_days BETWEEN 0 AND 730),
  invoice_status text NOT NULL DEFAULT 'not_invoiced' CHECK (invoice_status IN ('not_invoiced', 'invoiced', 'paid')),
  invoice_number text CHECK (length(invoice_number) BETWEEN 1 AND 50),
  invoiced_on    date,
  paid_on        date,
  UNIQUE (job_order_id, position),
  FOREIGN KEY (tenant_id, job_order_id) REFERENCES job_retainer (tenant_id, job_order_id) ON DELETE CASCADE,
  CONSTRAINT retainer_stage_due CHECK (
    (due_kind = 'on_date') = (due_on IS NOT NULL) AND (due_kind = 'on_date') = (due_days IS NULL)
  ),
  CONSTRAINT retainer_stage_invoice CHECK (
    (invoice_status = 'not_invoiced' AND invoice_number IS NULL AND invoiced_on IS NULL AND paid_on IS NULL)
    OR (invoice_status = 'invoiced' AND invoiced_on IS NOT NULL AND paid_on IS NULL)
    OR (invoice_status = 'paid' AND invoiced_on IS NOT NULL AND paid_on IS NOT NULL AND paid_on >= invoiced_on)
  )
);

ALTER TABLE job_retainer ENABLE ROW LEVEL SECURITY;
CREATE POLICY retainer_select ON job_retainer FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:read'));
CREATE POLICY retainer_insert ON job_retainer FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));
CREATE POLICY retainer_update ON job_retainer FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));
CREATE POLICY retainer_delete ON job_retainer FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));

ALTER TABLE job_retainer_stage ENABLE ROW LEVEL SECURITY;
CREATE POLICY stage_select ON job_retainer_stage FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:read'));
CREATE POLICY stage_insert ON job_retainer_stage FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));
CREATE POLICY stage_update ON job_retainer_stage FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));
CREATE POLICY stage_delete ON job_retainer_stage FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));

-- +goose Down
DROP TABLE IF EXISTS job_retainer_stage;
DROP TABLE IF EXISTS job_retainer;
