-- placements module: a candidate placed in a job (start date, guarantee
-- period, recruiter credited, invoice), and its salary and fee in their own
-- table that only permitted users can see.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE placement (
  id             text PRIMARY KEY DEFAULT new_id('plc') CHECK (id ~ '^plc_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  version        integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  application_id text NOT NULL,
  recruiter_id   text,
  start_date     date NOT NULL,
  guarantee_days integer NOT NULL CHECK (guarantee_days BETWEEN 0 AND 365),
  status         text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'fell_through', 'cancelled')),
  left_on        date,
  status_note    text CHECK (length(status_note) <= 1000),
  invoice_status text NOT NULL DEFAULT 'not_invoiced' CHECK (invoice_status IN ('not_invoiced', 'invoiced', 'paid')),
  invoice_number text CHECK (length(invoice_number) <= 100),
  invoiced_on    date,
  paid_on        date,
  created_by     text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, application_id), -- one placement per application
  FOREIGN KEY (tenant_id, application_id) REFERENCES application (tenant_id, id) ON DELETE CASCADE,
  -- A fall-through has the day they left, within the guarantee period.
  CONSTRAINT placement_left CHECK ((status = 'fell_through') = (left_on IS NOT NULL)),
  CONSTRAINT placement_left_in_guarantee CHECK (left_on IS NULL OR left_on BETWEEN start_date AND start_date + guarantee_days),
  -- Invoiced and paid carry their dates; not invoiced carries none.
  CONSTRAINT placement_invoice CHECK (
    (invoice_status = 'not_invoiced') = (invoiced_on IS NULL)
    AND (invoice_status = 'paid') = (paid_on IS NOT NULL)
    AND (invoice_status <> 'not_invoiced' OR invoice_number IS NULL)
  )
);
CREATE INDEX placement_start_idx ON placement (tenant_id, start_date DESC, id DESC);

ALTER TABLE placement ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON placement
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- The agreed salary and the fee: rows are invisible without
-- placements.fees:read, and only writable with placements.fees:update,
-- whatever the query.
CREATE TABLE placement_money (
  placement_id    text PRIMARY KEY,
  tenant_id       text NOT NULL,
  salary_amount   numeric(14, 2) CHECK (salary_amount >= 0),
  salary_currency text CHECK (salary_currency ~ '^[A-Z]{3}$'),
  salary_period   text CHECK (salary_period IN ('hour', 'day', 'month', 'year')),
  fee_percent     numeric(5, 2) CHECK (fee_percent > 0 AND fee_percent <= 100),
  fee_amount      numeric(14, 2) CHECK (fee_amount >= 0),
  fee_currency    text CHECK (fee_currency ~ '^[A-Z]{3}$'),
  updated_by      text NOT NULL,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, placement_id) REFERENCES placement (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT placement_money_salary CHECK (
    (salary_amount IS NULL) = (salary_currency IS NULL) AND (salary_amount IS NULL) = (salary_period IS NULL)
  ),
  CONSTRAINT placement_money_fee CHECK (
    (fee_amount IS NULL) = (fee_currency IS NULL) AND (fee_percent IS NULL OR (fee_amount IS NOT NULL AND salary_amount IS NOT NULL))
  ),
  CONSTRAINT placement_money_something CHECK (salary_amount IS NOT NULL OR fee_amount IS NOT NULL)
);

ALTER TABLE placement_money ENABLE ROW LEVEL SECURITY;
CREATE POLICY money_select ON placement_money FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:read'));
CREATE POLICY money_insert ON placement_money FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));
CREATE POLICY money_update ON placement_money FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));
CREATE POLICY money_delete ON placement_money FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:update'));

-- +goose Down
DROP TABLE IF EXISTS placement_money;
DROP TABLE IF EXISTS placement;
