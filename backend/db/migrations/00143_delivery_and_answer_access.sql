-- +goose Up
ALTER TABLE assistant_answer ADD COLUMN used_cv boolean NOT NULL DEFAULT false,
  ADD COLUMN market_scope text;
-- Older answers did not record which input sections they used.
UPDATE assistant_answer SET used_cv = true WHERE subject_type IN ('candidate', 'application');

ALTER TABLE placement_invoice ADD COLUMN delivery_email text,
  ADD COLUMN delivery_company text, ADD COLUMN delivery_to text, ADD COLUMN issued_number text;
CREATE UNIQUE INDEX placement_invoice_delivery ON placement_invoice(tenant_id, delivery_email)
  WHERE delivery_email IS NOT NULL;

-- Earlier releases queued a PDF with a deterministic attachment key, then
-- immediately marked the placement sent. Reconcile from the actual email.
UPDATE placement_invoice i
SET delivery_email = e.id, delivery_company = j.company_id, delivery_to = e.to_addresses[1], issued_number = p.invoice_number,
    sent_at = e.sent_at, send_error = CASE WHEN e.status = 'failed' THEN 'Invoice email delivery failed.' END
FROM placement p JOIN application ap ON (ap.tenant_id, ap.id) = (p.tenant_id, p.application_id)
JOIN job_order j ON (j.tenant_id, j.id) = (ap.tenant_id, ap.job_order_id)
JOIN email e ON (e.tenant_id, e.application_id) = (p.tenant_id, p.application_id)
JOIN email_attachment a ON (a.tenant_id, a.email_id) = (e.tenant_id, e.id)
WHERE (p.tenant_id, p.id) = (i.tenant_id, i.placement_id) AND i.sent_at IS NOT NULL AND e.fees_restricted
  AND a.storage_key = p.tenant_id || '/eat_invoice_' || p.id || '_' || regexp_replace(lower(p.invoice_number), '[^a-z0-9_]', '_', 'g');

CREATE TABLE placement_invoice_copy (
  tenant_id text NOT NULL, placement_id text NOT NULL, docx bytea NOT NULL, pdf bytea NOT NULL,
  PRIMARY KEY (tenant_id, placement_id),
  FOREIGN KEY (tenant_id, placement_id) REFERENCES placement_invoice(tenant_id, placement_id) ON DELETE CASCADE
);
ALTER TABLE placement_invoice_copy ENABLE ROW LEVEL SECURITY;
CREATE POLICY invoice_copy ON placement_invoice_copy
  USING (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:read')
         AND EXISTS (SELECT 1 FROM placement p WHERE p.id = placement_id))
  WITH CHECK (tenant_id = app_current_tenant() AND app_has_permission('placements.fees:read')
         AND EXISTS (SELECT 1 FROM placement p WHERE p.id = placement_id));

-- +goose Down
DROP TABLE placement_invoice_copy;
ALTER TABLE placement_invoice DROP COLUMN delivery_email, DROP COLUMN delivery_company, DROP COLUMN delivery_to, DROP COLUMN issued_number;
ALTER TABLE assistant_answer DROP COLUMN used_cv, DROP COLUMN market_scope;
