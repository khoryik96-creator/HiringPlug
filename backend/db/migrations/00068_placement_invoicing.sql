-- companies: a client's terms of business (company_terms), which fill in a
--   placement's fee, payment terms and guarantee.
-- placements: a placement's invoicing (placement_invoice): who receives the
--   invoice, the payment terms and due date (null: the start date plus the
--   terms), and whether it is emailed on the start date; and the agency's
--   invoice template (invoice_settings): its details, numbering, tax, wording
--   and the email that carries it.
-- Committed migrations are never edited (CI enforces this): add a new file.

-- +goose Up
CREATE TABLE company_terms (
  tenant_id          text NOT NULL,
  company_id         text NOT NULL,
  fee_percent        numeric(5, 2) CHECK (fee_percent > 0 AND fee_percent <= 100),
  payment_terms_days integer CHECK (payment_terms_days BETWEEN 0 AND 365),
  guarantee_days     integer CHECK (guarantee_days BETWEEN 0 AND 365),
  signed_on          date,
  notes              text CHECK (length(notes) <= 2000),
  version            integer NOT NULL DEFAULT 1,
  updated_by         text NOT NULL,
  updated_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, company_id),
  FOREIGN KEY (tenant_id, company_id) REFERENCES company (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE company_terms ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON company_terms
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE placement_invoice (
  tenant_id          text NOT NULL,
  placement_id       text NOT NULL,
  contact_id         text,
  payment_terms_days integer NOT NULL DEFAULT 30 CHECK (payment_terms_days BETWEEN 0 AND 365),
  due_on             date,
  auto_send          boolean NOT NULL DEFAULT false,
  sent_at            timestamptz,
  send_error         text CHECK (length(send_error) <= 500),
  updated_by         text NOT NULL,
  updated_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, placement_id),
  FOREIGN KEY (tenant_id, placement_id) REFERENCES placement (tenant_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tenant_id, contact_id) REFERENCES contact (tenant_id, id) ON DELETE SET NULL (contact_id)
);
ALTER TABLE placement_invoice ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON placement_invoice
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

CREATE TABLE invoice_settings (
  tenant_id     text PRIMARY KEY REFERENCES tenant (id),
  issuer        text NOT NULL CHECK (length(issuer) <= 1000),
  bank_details  text NOT NULL CHECK (length(bank_details) <= 1000),
  number_prefix text NOT NULL CHECK (length(number_prefix) <= 20),
  next_number   integer NOT NULL CHECK (next_number BETWEEN 1 AND 99999999),
  tax_label     text NOT NULL CHECK (length(tax_label) <= 40),
  tax_percent   numeric(5, 2) NOT NULL CHECK (tax_percent >= 0 AND tax_percent <= 100),
  line_text     text NOT NULL CHECK (length(line_text) BETWEEN 1 AND 1000),
  footer        text NOT NULL CHECK (length(footer) <= 2000),
  email_subject text NOT NULL CHECK (length(email_subject) BETWEEN 1 AND 200),
  email_body    text NOT NULL CHECK (length(email_body) BETWEEN 1 AND 10000),
  send_hour     integer NOT NULL CHECK (send_hour BETWEEN 0 AND 23),
  version       integer NOT NULL DEFAULT 1,
  updated_by    text NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE invoice_settings ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invoice_settings
  USING (tenant_id = app_current_tenant()) WITH CHECK (tenant_id = app_current_tenant());

-- +goose Down
DROP TABLE IF EXISTS invoice_settings;
DROP TABLE IF EXISTS placement_invoice;
DROP TABLE IF EXISTS company_terms;
