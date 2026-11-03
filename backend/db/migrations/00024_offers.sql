-- offers module: the terms offered to a candidate, their approval, and the
-- answer; the offered salary in its own table that only permitted users see.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE offer (
  id             text PRIMARY KEY DEFAULT new_id('ofr') CHECK (id ~ '^ofr_[0-9a-hjkmnp-tv-z]{26}$'),
  tenant_id      text NOT NULL REFERENCES tenant (id),
  version        integer NOT NULL DEFAULT 1 CHECK (version >= 1),
  application_id text NOT NULL,
  status         text NOT NULL DEFAULT 'draft'
                 CHECK (status IN ('draft', 'pending_approval', 'approved', 'sent', 'accepted', 'declined', 'withdrawn')),
  start_date     date,
  expires_on     date,
  terms          text CHECK (length(terms) <= 5000),
  requested_by   text,
  decided_by     text,
  decided_at     timestamptz,
  decision_note  text CHECK (length(decision_note) <= 1000),
  sent_at        timestamptz,
  answered_at    timestamptz,
  created_by     text NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, id),
  FOREIGN KEY (tenant_id, application_id) REFERENCES application (tenant_id, id) ON DELETE CASCADE,
  -- Approved and later steps were approved by someone; answers come after sending.
  CONSTRAINT offer_decided CHECK (status NOT IN ('approved', 'sent', 'accepted', 'declined') OR decided_at IS NOT NULL),
  CONSTRAINT offer_sent CHECK (status NOT IN ('sent', 'accepted', 'declined') OR sent_at IS NOT NULL),
  CONSTRAINT offer_answered CHECK ((status IN ('accepted', 'declined')) = (answered_at IS NOT NULL))
);
-- One open offer per application (a declined or withdrawn one can be followed by a new one).
CREATE UNIQUE INDEX offer_open_idx ON offer (tenant_id, application_id) WHERE status NOT IN ('declined', 'withdrawn');
CREATE INDEX offer_status_idx ON offer (tenant_id, status, id DESC);

ALTER TABLE offer ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON offer
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- The salary offered: rows are invisible without offers.salary:read, and only
-- writable with offers.salary:update, whatever the query.
CREATE TABLE offer_salary (
  offer_id   text PRIMARY KEY,
  tenant_id  text NOT NULL,
  amount     numeric(14, 2) NOT NULL CHECK (amount >= 0),
  currency   text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  period     text NOT NULL CHECK (period IN ('hour', 'day', 'month', 'year')),
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, offer_id) REFERENCES offer (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE offer_salary ENABLE ROW LEVEL SECURITY;
CREATE POLICY salary_select ON offer_salary FOR SELECT
  USING (tenant_id = app_current_tenant() AND app_has_permission('offers.salary:read'));
CREATE POLICY salary_insert ON offer_salary FOR INSERT
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('offers.salary:update'));
CREATE POLICY salary_update ON offer_salary FOR UPDATE
  USING (tenant_id = app_current_tenant() AND app_has_permission('offers.salary:update'))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('offers.salary:update'));
CREATE POLICY salary_delete ON offer_salary FOR DELETE
  USING (tenant_id = app_current_tenant() AND app_has_permission('offers.salary:update'));

-- +goose Down
DROP TABLE IF EXISTS offer_salary;
DROP TABLE IF EXISTS offer;
